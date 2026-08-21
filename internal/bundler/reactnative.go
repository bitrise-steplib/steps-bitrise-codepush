package bundler

import (
	"fmt"
	"os"
	"path/filepath"
)

type bundlePaths struct {
	outputDir  string
	bundlePath string
	assetsDir  string
}

// ReactNativeBundler bundles using "npx react-native bundle" (Metro bundler).
type ReactNativeBundler struct {
	executor CommandExecutor
	logger   Logger
}

func (b *ReactNativeBundler) Bundle(config *ProjectConfig, opts *BundleOptions) (*BundleResult, error) {
	outputDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("resolving output directory: %w", err)
	}

	// The Metro bundler copies assets into an "assets" subdirectory of --assets-dest,
	// so the destination is the output directory itself (yielding <outputDir>/assets/...).
	// Passing <outputDir>/assets here would nest them one level too deep. This matches
	// the Expo bundler and the layout the CodePush SDK expects.
	assetsDir := outputDir
	if err := ensureDir(assetsDir); err != nil {
		return nil, err
	}

	bundleName := opts.BundleName
	if bundleName == "" {
		bundleName = DefaultBundleName(opts.Platform)
	}

	bundlePath := filepath.Join(outputDir, bundleName)

	paths := bundlePaths{
		outputDir:  outputDir,
		bundlePath: bundlePath,
		assetsDir:  assetsDir,
	}
	args := b.buildArgs(config, opts, paths)

	b.logger.Infof("Bundling %s (react-native)", opts.Platform)
	if err := b.executor.Run(config.ProjectDir, os.Stdout, os.Stderr, "npx", args...); err != nil {
		return nil, fmt.Errorf("react-native bundle failed: %w", err)
	}

	if _, err := os.Stat(bundlePath); err != nil {
		return nil, fmt.Errorf("bundle file was not created at %s", bundlePath)
	}

	result := &BundleResult{
		BundlePath:  bundlePath,
		AssetsDir:   assetsDir,
		OutputDir:   outputDir,
		ProjectType: ProjectTypeReactNative,
		Platform:    opts.Platform,
	}

	return result, nil
}

func (b *ReactNativeBundler) buildArgs(config *ProjectConfig, opts *BundleOptions, paths bundlePaths) []string {
	entryFile := opts.EntryFile
	if entryFile == "" {
		entryFile = config.EntryFile
	}

	devStr := "false"
	if opts.Dev {
		devStr = "true"
	}

	args := []string{
		"react-native", "bundle",
		"--entry-file", entryFile,
		"--platform", string(opts.Platform),
		"--dev", devStr,
		"--bundle-output", paths.bundlePath,
		"--assets-dest", paths.assetsDir,
	}

	if opts.ResetCache {
		args = append(args, "--reset-cache")
	}

	metroConfig := opts.MetroConfig
	if metroConfig == "" {
		metroConfig = config.MetroConfig
	}
	if metroConfig != "" {
		args = append(args, "--config", metroConfig)
	}

	args = append(args, opts.ExtraBundlerOpts...)

	return args
}
