package bundler

import (
	"fmt"
	"os"
	"path/filepath"
)

type HermesCompiler struct {
	executor CommandExecutor
	logger   Logger
}

func NewHermesCompiler(executor CommandExecutor, logger Logger) *HermesCompiler {
	return &HermesCompiler{executor: executor, logger: logger}
}

// The compiled bytecode replaces the original bundle file (CodePush clients expect the original
// filename).
func (h *HermesCompiler) Compile(hermescPath string, bundlePath string, sourcemapPath string, extraHermesFlags []string) error {
	if _, err := os.Stat(hermescPath); err != nil {
		return fmt.Errorf("hermesc binary not found at %s: %w", hermescPath, err)
	}

	if _, err := os.Stat(bundlePath); err != nil {
		return fmt.Errorf("bundle file not found at %s: %w", bundlePath, err)
	}

	hbcPath := bundlePath + ".hbc"

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

	if err := os.Rename(hbcPath, bundlePath); err != nil {
		return fmt.Errorf("replacing bundle with Hermes bytecode: %w", err)
	}

	if sourcemapPath != "" {
		hermesMapPath := hbcPath + ".map"
		if _, err := os.Stat(hermesMapPath); err == nil {
			h.composeSourceMaps(bundlePath, sourcemapPath, hermesMapPath)
		}
	}

	return nil
}

func (h *HermesCompiler) composeSourceMaps(bundlePath string, metroMapPath string, hermesMapPath string) {
	projectDir := filepath.Dir(bundlePath)

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

	if err := os.Rename(composedPath, metroMapPath); err != nil {
		h.logger.Warnf("could not replace source map with composed version: %v", err)
	}
	if err := os.Remove(hermesMapPath); err != nil {
		h.logger.Warnf("could not clean up Hermes source map: %v", err)
	}
}
