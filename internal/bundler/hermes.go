// Ported from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/bundler/hermes.go). The interactive output.Writer dependency was replaced with the
// package-local Logger interface; behavior is otherwise unchanged.
package bundler

import (
	"fmt"
	"os"
	"path/filepath"
)

// HermesCompiler handles Hermes bytecode compilation of JS bundles.
type HermesCompiler struct {
	executor CommandExecutor
	logger   Logger
}

// NewHermesCompiler creates a new HermesCompiler.
func NewHermesCompiler(executor CommandExecutor, logger Logger) *HermesCompiler {
	return &HermesCompiler{executor: executor, logger: logger}
}

// Compile takes a JS bundle path and compiles it to Hermes bytecode.
// The compiled bytecode replaces the original bundle file (CodePush clients
// expect the original filename).
// If sourcemapPath is non-empty, attempts to compose source maps.
// extraHermesFlags are appended to the hermesc invocation before the input file.
func (h *HermesCompiler) Compile(hermescPath string, bundlePath string, sourcemapPath string, extraHermesFlags []string) error {
	if _, err := os.Stat(hermescPath); err != nil {
		return fmt.Errorf("hermesc binary not found at %s: %w", hermescPath, err)
	}

	if _, err := os.Stat(bundlePath); err != nil {
		return fmt.Errorf("bundle file not found at %s: %w", bundlePath, err)
	}

	hbcPath := bundlePath + ".hbc"

	// Compile JS to Hermes bytecode
	args := []string{"-emit-binary", "-out", hbcPath}

	if sourcemapPath != "" {
		args = append(args, "-output-source-map")
	}

	args = append(args, extraHermesFlags...)
	args = append(args, bundlePath)

	h.logger.Debugf("Running Hermes compilation: %s %v", hermescPath, args)

	if err := h.executor.Run("", os.Stderr, os.Stderr, hermescPath, args...); err != nil {
		return fmt.Errorf("hermes compilation failed: %w", err)
	}

	// Replace the original JS bundle with the compiled bytecode
	if err := os.Rename(hbcPath, bundlePath); err != nil {
		return fmt.Errorf("replacing bundle with Hermes bytecode: %w", err)
	}

	// Compose source maps if both metro and hermes source maps exist
	if sourcemapPath != "" {
		hermesMapPath := hbcPath + ".map"
		if _, err := os.Stat(hermesMapPath); err == nil {
			h.composeSourceMaps(bundlePath, sourcemapPath, hermesMapPath)
		}
	}

	return nil
}

// composeSourceMaps attempts to compose Metro and Hermes source maps.
// This is a best-effort operation; failures are logged but not fatal.
func (h *HermesCompiler) composeSourceMaps(bundlePath string, metroMapPath string, hermesMapPath string) {
	projectDir := filepath.Dir(bundlePath)

	// Look for the compose-source-maps script
	composeScript := filepath.Join(projectDir, "node_modules", "react-native", "scripts", "compose-source-maps.js")
	if _, err := os.Stat(composeScript); err != nil {
		h.logger.Warnf("compose-source-maps.js not found, using Hermes source map only")
		if err := os.Rename(hermesMapPath, metroMapPath); err != nil {
			h.logger.Warnf("could not rename Hermes source map: %v", err)
		}
		return
	}

	composedPath := metroMapPath + ".composed"
	err := h.executor.Run("", os.Stderr, os.Stderr, "node", composeScript, metroMapPath, hermesMapPath, "-o", composedPath)
	if err != nil {
		h.logger.Warnf("source map composition failed, using Hermes source map only")
		if err := os.Rename(hermesMapPath, metroMapPath); err != nil {
			h.logger.Warnf("could not rename Hermes source map: %v", err)
		}
		return
	}

	// Replace original sourcemap with composed one
	if err := os.Rename(composedPath, metroMapPath); err != nil {
		h.logger.Warnf("could not replace source map with composed version: %v", err)
	}
	if err := os.Remove(hermesMapPath); err != nil {
		h.logger.Warnf("could not clean up Hermes source map: %v", err)
	}
}
