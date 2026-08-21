package bundler

import (
	"fmt"
	"os"
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
func (h *HermesCompiler) Compile(hermescPath string, bundlePath string, extraHermesFlags []string) error {
	if _, err := os.Stat(hermescPath); err != nil {
		return fmt.Errorf("hermesc binary not found at %s: %w", hermescPath, err)
	}

	if _, err := os.Stat(bundlePath); err != nil {
		return fmt.Errorf("bundle file not found at %s: %w", bundlePath, err)
	}

	hbcPath := bundlePath + ".hbc"

	args := []string{"-emit-binary", "-out", hbcPath}
	args = append(args, extraHermesFlags...)
	args = append(args, bundlePath)

	h.logger.Debugf("Running Hermes compilation: %s %v", hermescPath, args)

	if err := h.executor.Run("", os.Stderr, os.Stderr, hermescPath, args...); err != nil {
		return fmt.Errorf("hermes compilation failed: %w", err)
	}

	if err := os.Rename(hbcPath, bundlePath); err != nil {
		return fmt.Errorf("replacing bundle with Hermes bytecode: %w", err)
	}

	return nil
}
