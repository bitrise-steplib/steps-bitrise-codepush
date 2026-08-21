package bundler

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// ExpoBundler bundles using "npx expo export:embed" for Expo-managed projects.
// export:embed uses the same Metro+Hermes pipeline as the native app build,
// producing a bundle the CodePush SDK can load directly.
type ExpoBundler struct {
	executor CommandExecutor
	logger   Logger
}

func (b *ExpoBundler) Bundle(config *ProjectConfig, opts *BundleOptions) (*BundleResult, error) {
	outputDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("resolving output directory: %w", err)
	}

	if err := ensureDir(outputDir); err != nil {
		return nil, err
	}

	bundleName := resolveExpoBundleName(config, opts)
	bundlePath := filepath.Join(outputDir, bundleName)

	args := b.buildArgs(config, opts, outputDir, bundlePath)

	b.logger.Infof("Bundling %s (expo)", opts.Platform)
	if err := b.executor.Run(config.ProjectDir, os.Stdout, os.Stderr, "npx", args...); err != nil {
		return nil, fmt.Errorf("expo export:embed failed: %w", err)
	}

	result := &BundleResult{
		BundlePath: bundlePath,
		AssetsDir:  outputDir,
		OutputDir:  outputDir,
		// HermesApplied mirrors config.HermesEnabled: when true, --bytecode was passed
		// to expo export:embed, which manages Hermes internally (unlike the RN path where
		// hermesc runs as a separate post-bundle step).
		HermesApplied: config.HermesEnabled,
		ProjectType:   ProjectTypeExpo,
		Platform:      opts.Platform,
	}

	return result, nil
}

func (b *ExpoBundler) buildArgs(config *ProjectConfig, opts *BundleOptions, outputDir, bundlePath string) []string {
	args := []string{
		"expo", "export:embed",
		"--entry-file", config.EntryFile,
		"--platform", string(opts.Platform),
		"--bundle-output", bundlePath,
		"--assets-dest", outputDir,
		"--dev", strconv.FormatBool(opts.Dev),
		"--minify", strconv.FormatBool(opts.Minify),
	}

	if opts.ResetCache {
		args = append(args, "--reset-cache")
	}

	if config.HermesEnabled {
		args = append(args, "--bytecode")
	}

	args = append(args, opts.ExtraBundlerOpts...)

	return args
}

func resolveExpoBundleName(config *ProjectConfig, opts *BundleOptions) string {
	if opts.BundleName != "" {
		return opts.BundleName
	}
	if config.BundleName != "" {
		return config.BundleName
	}
	return DefaultBundleName(config.Platform)
}
