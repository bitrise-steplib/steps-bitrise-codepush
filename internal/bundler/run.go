package bundler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Run executes the full bundle pipeline:
// 1. Detect project configuration
// 2. Execute the appropriate bundler
// 3. Compile with Hermes if applicable
func Run(opts *BundleOptions, logger Logger) (*BundleResult, error) {
	return RunWithExecutor(opts, &DefaultExecutor{}, logger)
}

// RunWithExecutor executes the full bundle pipeline with the given executor.
// This allows tests to provide a mock executor.
func RunWithExecutor(opts *BundleOptions, executor CommandExecutor, logger Logger) (*BundleResult, error) {
	hermesMode, err := resolveRunOptions(opts)
	if err != nil {
		return nil, err
	}

	if !opts.SkipInstall {
		if err := installDependencies(opts.ProjectDir, executor, logger); err != nil {
			return nil, err
		}
	}

	config, err := DetectProject(opts.ProjectDir, opts.Platform, hermesMode, opts)
	if err != nil {
		return nil, err
	}

	if opts.EntryFile != "" {
		config.EntryFile = opts.EntryFile
	}
	if opts.MetroConfig != "" {
		config.MetroConfig = opts.MetroConfig
	}

	bundler, err := NewBundler(config.ProjectType, executor, logger)
	if err != nil {
		return nil, err
	}

	result, err := bundler.Bundle(config, opts)
	if err != nil {
		return nil, err
	}

	if err := compileWithHermes(config, result, opts.ExtraHermesFlags, executor, logger); err != nil {
		return nil, err
	}

	return result, nil
}

func resolveRunOptions(opts *BundleOptions) (HermesMode, error) {
	projectDir := opts.ProjectDir
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting current directory: %w", err)
		}
		projectDir = cwd
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolving project directory: %w", err)
	}
	opts.ProjectDir = absProjectDir

	if opts.OutputDir == "" {
		opts.OutputDir = DefaultOutputDir
	}

	if opts.SourcemapOutput != "" {
		opts.Sourcemap = true
	}

	hermesMode := opts.HermesMode
	if hermesMode == "" {
		hermesMode = HermesModeAuto
	}
	return hermesMode, nil
}

func compileWithHermes(config *ProjectConfig, result *BundleResult, extraFlags []string, executor CommandExecutor, logger Logger) error {
	if !config.HermesEnabled || config.ProjectType != ProjectTypeReactNative {
		return nil
	}
	if config.HermescPath == "" {
		return errors.New("hermes is enabled but hermesc was not found in node_modules: run 'npm install' or set the hermes input to 'off'")
	}

	compiler := NewHermesCompiler(executor, logger)
	if err := compiler.Compile(config.HermescPath, result.BundlePath, result.SourcemapPath, extraFlags); err != nil {
		return err
	}
	result.HermesApplied = true
	return nil
}
