package step

import (
	"fmt"
	"path/filepath"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-steplib/steps-bitrise-codepush/internal/bundler"
	ziputil "github.com/bitrise-steplib/steps-bitrise-codepush/internal/zip"
)

const outputPackagePath = "BITRISE_CODEPUSH_PACKAGE_PATH"

type Config struct {
	ProjectDir            string `env:"project_dir,dir"`
	Platform              string `env:"platform,opt[ios,android]"`
	EntryFile             string `env:"entry_file"`
	BundleName            string `env:"bundle_name"`
	HermesMode            string `env:"hermes,opt[auto,on,off]"`
	SkipDependencyInstall bool   `env:"skip_dependency_install,opt[true,false]"`

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

	return cfg, nil
}

func (s Step) Run(cfg Config) (Result, error) {
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
