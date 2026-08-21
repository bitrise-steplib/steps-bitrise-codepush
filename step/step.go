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
)

const (
	outputUpdateID    = "BITRISE_CODEPUSH_UPDATE_ID"
	outputPackagePath = "BITRISE_CODEPUSH_PACKAGE_PATH"
)

const defaultServerURL = "https://api.bitrise.io"

const codePushAPIPath = "/release-management/v2/code-push/v1"

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

	AppVersion          string  `env:"app_version,required"`
	Description         string  `env:"description"`
	RolloutPercentage   float64 `env:"rollout_percentage"`
	Mandatory           bool    `env:"mandatory,opt[true,false]"`
	DisabledAfterUpload bool    `env:"disabled_after_upload,opt[true,false]"`

	DeployDir string `env:"BITRISE_DEPLOY_DIR,dir"`

	VerboseLog bool `env:"verbose_log,opt[true,false]"`
}

type Result struct {
	UpdateID    string
	Status      string
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

	if cfg.RolloutPercentage == 0 {
		cfg.RolloutPercentage = 100
	}
	if cfg.RolloutPercentage < 0 || cfg.RolloutPercentage > 100 {
		return Config{}, fmt.Errorf("rollout_percentage must be a number between 0 and 100, got %g", cfg.RolloutPercentage)
	}

	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultServerURL
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")

	stepconf.Print(cfg)
	s.logger.Println()

	return cfg, nil
}

// ResolveDeployment runs before bundling so a bad credential/deployment fails fast; codepush.Push
// resolves it again internally as part of its upload flow, which is harmless (one extra API call).
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

	s.logger.Println()
	s.logger.Infof("Pushing update to CodePush")
	pushResult, err := codepush.Push(ctx, client, &codepush.PushOptions{
		AppID:        cfg.AppID,
		DeploymentID: cfg.Deployment,
		Token:        string(cfg.APIToken),
		AppVersion:   cfg.AppVersion,
		Description:  cfg.Description,
		Mandatory:    cfg.Mandatory,
		Disabled:     cfg.DisabledAfterUpload,
		Rollout:      cfg.RolloutPercentage,
		BundlePath:   bundleResult.OutputDir,
	}, s.logger)
	if err != nil {
		return Result{}, fmt.Errorf("push failed: %w", err)
	}
	s.logger.Donef("Push successful (update: %s, status: %s)", pushResult.UpdateID, pushResult.Status)

	return Result{
		UpdateID:    pushResult.UpdateID,
		Status:      pushResult.Status,
		PackagePath: pushResult.PackagePath,
		DeployDir:   cfg.DeployDir,
	}, nil
}

// The package is copied under $BITRISE_DEPLOY_DIR so a subsequent "Deploy to Bitrise.io" step
// picks it up automatically.
func (s Step) ExportOutputs(result Result) error {
	if err := s.exporter.ExportOutput(outputUpdateID, result.UpdateID); err != nil {
		return fmt.Errorf("exporting %s: %w", outputUpdateID, err)
	}

	dest := filepath.Join(result.DeployDir, filepath.Base(result.PackagePath))
	if err := s.exporter.ExportOutputFile(outputPackagePath, result.PackagePath, dest); err != nil {
		return fmt.Errorf("exporting %s: %w", outputPackagePath, err)
	}
	s.logger.Donef("Package available at: %s", dest)

	return nil
}
