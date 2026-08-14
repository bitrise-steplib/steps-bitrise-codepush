// Ported from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/bundler/expo.go). The PTY-backed invocation path and interactive output.Writer
// dependency were dropped for the same reason as internal/bundler/reactnative.go.
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

// Bundle implements Bundler for Expo projects.
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

	var mapPath string
	if opts.Sourcemap || opts.SourcemapOutput != "" {
		mapPath = sourcemapPathForExpo(opts, bundlePath)
		if opts.SourcemapOutput != "" {
			if err := ensureDir(filepath.Dir(mapPath)); err != nil {
				return nil, fmt.Errorf("creating sourcemap output directory: %w", err)
			}
		}
	}

	args := b.buildArgs(config, opts, outputDir, bundlePath, mapPath)

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

	if mapPath != "" {
		if _, err := os.Stat(mapPath); err == nil {
			result.SourcemapPath = mapPath
		}
	}

	return result, nil
}

// buildArgs constructs the argument list for "npx expo export:embed".
func (b *ExpoBundler) buildArgs(config *ProjectConfig, opts *BundleOptions, outputDir, bundlePath, mapPath string) []string {
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

	if mapPath != "" {
		args = append(args, "--sourcemap-output", mapPath)
	}

	args = append(args, opts.ExtraBundlerOpts...)

	return args
}

// resolveExpoBundleName returns the bundle filename the CodePush SDK expects to find
// in the zip. Priority: opts.BundleName (--bundle-name flag) > config.BundleName
// (auto-detected from native project files) > DefaultBundleName.
func resolveExpoBundleName(config *ProjectConfig, opts *BundleOptions) string {
	if opts.BundleName != "" {
		return opts.BundleName
	}
	if config.BundleName != "" {
		return config.BundleName
	}
	return DefaultBundleName(config.Platform)
}

// sourcemapPathForExpo returns the sourcemap output path for expo export:embed.
// If SourcemapOutput is explicitly set, that path is used (resolved to absolute
// against ProjectDir if relative); otherwise the map is placed next to the
// bundle at bundlePath+".map".
func sourcemapPathForExpo(opts *BundleOptions, bundlePath string) string {
	if opts.SourcemapOutput != "" {
		if filepath.IsAbs(opts.SourcemapOutput) {
			return opts.SourcemapOutput
		}
		return filepath.Join(opts.ProjectDir, opts.SourcemapOutput)
	}
	return bundlePath + ".map"
}
