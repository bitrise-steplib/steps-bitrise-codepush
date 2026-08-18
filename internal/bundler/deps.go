package bundler

import (
	"fmt"
	"os"
	"path/filepath"
)

func detectPackageManager(projectDir string) (name, cmd string) {
	lockFiles := []struct {
		file string
		name string
		cmd  string
	}{
		{"yarn.lock", "yarn", "yarn"},
		{"pnpm-lock.yaml", "pnpm", "pnpm"},
		{"bun.lockb", "bun", "bun"},
		{"bun.lock", "bun", "bun"},
	}

	for _, lf := range lockFiles {
		if _, err := os.Stat(filepath.Join(projectDir, lf.file)); err == nil {
			return lf.name, lf.cmd
		}
	}

	return "npm", "npm"
}

func installDependencies(projectDir string, executor CommandExecutor, logger Logger) error {
	name, cmd := detectPackageManager(projectDir)

	logger.Infof("Installing dependencies (%s)", name)
	if err := executor.Run(projectDir, os.Stdout, os.Stderr, cmd, "install"); err != nil {
		return fmt.Errorf("installing dependencies with %s failed: %w", name, err)
	}
	return nil
}
