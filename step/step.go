// Package step implements the Bitrise CodePush step following the ideal Bitrise Step
// architecture: ProcessConfig -> InstallDependencies -> Run -> ExportOutputs.
//
// The actual publishing logic (JS bundling, Hermes compilation, CodePush upload) is ported from
// bitrise-plugins-codepush-cli into internal/bundler and internal/codepush; see those packages'
// doc comments for details. This file only wires step inputs to that logic and exports outputs.
package step

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-steplib/steps-bitrise-codepush/internal/bundler"
	"github.com/bitrise-steplib/steps-bitrise-codepush/internal/codepush"
)

// defaultServerURL is the default Bitrise API server base URL.
const defaultServerURL = "https://api.bitrise.io"

// codePushAPIPath is appended to the server base URL to form the CodePush API base URL.
// Ported from bitrise-plugins-codepush-cli's internal/cmdutil.APIURL.
const codePushAPIPath = "/release-management/v2/code-push/v1"

// stepUserAgentVersion identifies this step's requests to the CodePush API. It is cosmetic
// (server-side logging/debugging only) and intentionally not wired to the step's own semver.
const stepUserAgentVersion = "1"

// Output env var keys exported by this step.
const (
	outputUpdateID    = "BITRISE_CODEPUSH_UPDATE_ID"
	outputAppVersion  = "BITRISE_CODEPUSH_APP_VERSION"
	outputPackagePath = "BITRISE_CODEPUSH_PACKAGE_PATH"
)

// Config is the parsed and validated set of step inputs.
type Config struct {
	// Bundling
	ProjectDir            string `env:"project_dir,dir"`
	Platform              string `env:"platform,opt[ios,android]"`
	EntryFile             string `env:"entry_file"`
	BundleName            string `env:"bundle_name"`
	HermesMode            string `env:"hermes,opt[auto,on,off]"`
	SkipDependencyInstall bool   `env:"skip_dependency_install,opt[true,false]"`

	// CodePush release
	AppID               string `env:"app_id,required"`
	Deployment          string `env:"deployment,required"`
	AppVersion          string `env:"app_version,required"`
	Description         string `env:"description"`
	RolloutPercentage   string `env:"rollout_percentage"`
	Mandatory           bool   `env:"mandatory,opt[true,false]"`
	DisabledAfterUpload bool   `env:"disabled_after_upload,opt[true,false]"`

	// Auth / advanced
	APIToken  stepconf.Secret `env:"api_token,required"`
	ServerURL string          `env:"server_url"`

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
	UpdateID    string
	AppVersion  string
	Status      string
	Rollout     float64
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

	if cfg.RolloutPercentage == "" {
		cfg.RolloutPercentage = "100"
	}
	if rollout, err := strconv.ParseFloat(cfg.RolloutPercentage, 64); err != nil || rollout < 0 || rollout > 100 {
		return Config{}, fmt.Errorf("rollout_percentage must be a number between 0 and 100, got %q", cfg.RolloutPercentage)
	}

	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultServerURL
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")

	return cfg, nil
}

// InstallDependencies is a no-op: the only external dependency this step needs (the target JS
// project's own package manager install) is part of the bundling business logic and runs in Run,
// not a step-level tooling dependency. Present to keep the step's shape aligned with the ideal
// architecture's 4 phases.
func (s Step) InstallDependencies() error {
	return nil
}

// Run bundles the JavaScript project and pushes the resulting package to CodePush.
func (s Step) Run(cfg Config) (Result, error) {
	ctx := context.Background()

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

	if err := bundler.ValidatePlatform(bundleOpts.Platform); err != nil {
		return Result{}, err
	}
	if err := bundler.ValidateHermesMode(bundleOpts.HermesMode); err != nil {
		return Result{}, err
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

	// Already validated in ProcessConfig.
	rollout, _ := strconv.ParseFloat(cfg.RolloutPercentage, 64)

	client := codepush.NewHTTPClient(cfg.ServerURL+codePushAPIPath, string(cfg.APIToken), stepUserAgentVersion)

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
		Rollout:      rollout,
		BundlePath:   bundleResult.OutputDir,
	}, s.logger)
	if err != nil {
		return Result{}, fmt.Errorf("push failed: %w", err)
	}
	s.logger.Donef("Push successful (update: %s, status: %s)", pushResult.UpdateID, pushResult.Status)

	return Result{
		UpdateID:    pushResult.UpdateID,
		AppVersion:  pushResult.AppVersion,
		Status:      pushResult.Status,
		Rollout:     pushResult.Rollout,
		PackagePath: pushResult.PackagePath,
		DeployDir:   cfg.DeployDir,
	}, nil
}

// ExportOutputs exports the step outputs and copies the built package under $BITRISE_DEPLOY_DIR,
// so a subsequent "Deploy to Bitrise.io" step picks it up automatically (see the project brief,
// Phase 1 item 2).
func (s Step) ExportOutputs(result Result) error {
	if err := s.exporter.ExportOutput(outputUpdateID, result.UpdateID); err != nil {
		return fmt.Errorf("exporting %s: %w", outputUpdateID, err)
	}
	if err := s.exporter.ExportOutput(outputAppVersion, result.AppVersion); err != nil {
		return fmt.Errorf("exporting %s: %w", outputAppVersion, err)
	}

	if result.PackagePath != "" && result.DeployDir != "" {
		dest := filepath.Join(result.DeployDir, filepath.Base(result.PackagePath))
		if err := s.exporter.ExportOutputFile(outputPackagePath, result.PackagePath, dest); err != nil {
			return fmt.Errorf("exporting %s: %w", outputPackagePath, err)
		}
		s.logger.Donef("Package available at: %s", dest)
	}

	return nil
}
