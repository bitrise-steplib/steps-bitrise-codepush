package step

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-steplib/steps-bitrise-codepush/internal/bundler"
	"github.com/bitrise-steplib/steps-bitrise-codepush/internal/codepush"
	ziputil "github.com/bitrise-steplib/steps-bitrise-codepush/internal/zip"
)

const outputPackagePath = "BITRISE_CODEPUSH_PACKAGE_PATH"

const defaultServerURL = "https://api.bitrise.io"

const codePushAPIPath = "/release-management/v2/code-push/v1"

// stepUserAgentVersion identifies this step's requests to the CodePush API for server-side
// logging/debugging: the VCS revision the running binary was built from, so a specific build can
// be traced back to its source. Falls back to "unknown" if build info isn't available.
func stepUserAgentVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return "unknown"
}

type Config struct {
	ProjectDir            string `env:"project_dir,dir"`
	Platform              string `env:"platform,opt[ios,android]"`
	EntryFile             string `env:"entry_file"`
	BundleName            string `env:"bundle_name"`
	HermesMode            string `env:"hermes,opt[auto,on,off]"`
	SkipDependencyInstall bool   `env:"skip_dependency_install,opt[true,false]"`

	AppID      string          `env:"app_id,required"`
	Deployment string          `env:"deployment,required"`
	APIToken   stepconf.Secret `env:"api_token,required"`
	ServerURL  string          `env:"server_url"`

	DeployDir string `env:"BITRISE_DEPLOY_DIR,dir"`

	VerboseLog bool `env:"verbose_log,opt[true,false]"`
}

type Result struct {
	PackagePath string
	DeployDir   string
}

type Step struct {
	inputParser stepconf.InputParser
	logger      log.Logger
	exporter    export.Exporter
}

func New(inputParser stepconf.InputParser, logger log.Logger, exporter export.Exporter) Step {
	return Step{
		inputParser: inputParser,
		logger:      logger,
		exporter:    exporter,
	}
}

func (s Step) ProcessConfig() (Config, error) {
	var cfg Config
	if err := s.inputParser.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("issue with input: %w", err)
	}

	s.logger.EnableDebugLog(cfg.VerboseLog)
	stepconf.Print(cfg)
	s.logger.Println()

	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultServerURL
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")

	return cfg, nil
}

// Run resolves and validates the app/deployment/token against the CodePush API, then bundles the
// JavaScript project and packages it into a zip. Resolution runs first so a bad credential or
// deployment name fails fast, before spending time bundling.
func (s Step) Run(cfg Config) (Result, error) {
	ctx := context.Background()

	client := codepush.NewHTTPClient(cfg.ServerURL+codePushAPIPath, string(cfg.APIToken), stepUserAgentVersion())

	s.logger.Infof("Resolving CodePush app and deployment")
	if _, err := codepush.ResolveDeployment(ctx, client, cfg.AppID, cfg.Deployment, s.logger); err != nil {
		return Result{}, fmt.Errorf("resolving deployment: %w", err)
	}
	s.logger.Donef("App and deployment resolved")

	bundleOpts := &bundler.BundleOptions{
		Platform:    bundler.Platform(cfg.Platform),
		EntryFile:   cfg.EntryFile,
		OutputDir:   bundler.DefaultOutputDir,
		BundleName:  cfg.BundleName,
		ResetCache:  true,
		Sourcemap:   false,
		HermesMode:  bundler.HermesMode(cfg.HermesMode),
		ProjectDir:  cfg.ProjectDir,
		SkipInstall: cfg.SkipDependencyInstall,
	}

	s.logger.Println()
	s.logger.Infof("Bundling JavaScript")
	bundleResult, err := bundler.Run(bundleOpts, s.logger)
	if err != nil {
		return Result{}, fmt.Errorf("bundling failed: %w", err)
	}
	s.logger.Donef("Bundle created at %s", bundleResult.OutputDir)
	if bundleResult.HermesApplied {
		s.logger.Printf("Hermes bytecode compilation applied")
	}

	s.logger.Infof("Packaging bundle")
	zipPath, err := ziputil.Directory(bundleResult.OutputDir)
	if err != nil {
		return Result{}, fmt.Errorf("packaging bundle: %w", err)
	}

	return Result{
		PackagePath: zipPath,
		DeployDir:   cfg.DeployDir,
	}, nil
}

// ExportOutputs exports the built package under $BITRISE_DEPLOY_DIR, so a subsequent "Deploy to
// Bitrise.io" step picks it up automatically.
func (s Step) ExportOutputs(result Result) error {
	dest := filepath.Join(result.DeployDir, filepath.Base(result.PackagePath))
	if err := s.exporter.ExportOutputFile(outputPackagePath, result.PackagePath, dest); err != nil {
		return fmt.Errorf("exporting %s: %w", outputPackagePath, err)
	}
	s.logger.Donef("Package available at: %s", dest)

	return nil
}
