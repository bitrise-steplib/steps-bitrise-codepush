// Package step implements the Bitrise CodePush step.
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

// outputPackagePath is the output env var key exported by this step.
const outputPackagePath = "BITRISE_CODEPUSH_PACKAGE_PATH"

// Config is the parsed and validated set of step inputs.
type Config struct {
	// Bundling
	ProjectDir            string `env:"project_dir,dir"`
	Platform              string `env:"platform,opt[ios,android]"`
	EntryFile             string `env:"entry_file"`
	BundleName            string `env:"bundle_name"`
	HermesMode            string `env:"hermes,opt[auto,on,off]"`
	SkipDependencyInstall bool   `env:"skip_dependency_install,opt[true,false]"`

	// Populated from the Bitrise-provided build env, not a step input.
	// Note: stepconf only supports a single constraint token per field (see
	// stepconf.validateConstraint), so this cannot be "required,dir" — that combination never
	// matches any recognized constraint and would make ProcessConfig fail unconditionally. "dir"
	// alone already rejects an empty/missing value (os.Stat("") errors), which covers the
	// "required" intent in practice.
	DeployDir string `env:"BITRISE_DEPLOY_DIR,dir"`

	VerboseLog bool `env:"verbose_log,opt[true,false]"`
}

// Result is the outcome of a successful Run, consumed by ExportOutputs.
type Result struct {
	PackagePath string
	DeployDir   string
}

// Step implements the CodePush publish step.
type Step struct {
	inputParser stepconf.InputParser
	logger      log.Logger
	exporter    export.Exporter
}

// New creates a Step with its dependencies injected.
func New(inputParser stepconf.InputParser, logger log.Logger, exporter export.Exporter) Step {
	return Step{
		inputParser: inputParser,
		logger:      logger,
		exporter:    exporter,
	}
}

// ProcessConfig parses and validates the step's inputs.
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

// Run bundles the JavaScript project and packages it into a zip.
func (s Step) Run(cfg Config) (Result, error) {
	bundleOpts := &bundler.BundleOptions{
		Platform:    bundler.Platform(cfg.Platform),
		EntryFile:   cfg.EntryFile,
		OutputDir:   bundler.DefaultOutputDir,
		BundleName:  cfg.BundleName,
		ResetCache:  true,
		Sourcemap:   true,
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
