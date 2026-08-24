package bundler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWithExecutor(t *testing.T) {
	t.Run("react native project without hermes", func(t *testing.T) {
		dir := t.TempDir()
		outputDir := filepath.Join(dir, "output")

		// Set up a minimal React Native project
		writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies": {"react-native": "0.72.0"}}`)
		writeFile(t, filepath.Join(dir, "index.js"), "console.log('hello')")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			// Create the expected bundle output when npx is called
			for i, arg := range args {
				if arg == "--bundle-output" && i+1 < len(args) {
					require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o755))
					require.NoError(t, os.WriteFile(args[i+1], []byte("bundle"), 0o644))
				}
			}
		}

		opts := &BundleOptions{
			Platform:   PlatformIOS,
			ProjectDir: dir,
			OutputDir:  outputDir,
			HermesMode: HermesModeOff,
		}

		result, err := RunWithExecutor(opts, executor, newTestLogger())
		require.NoError(t, err)

		assert.Equal(t, ProjectTypeReactNative, result.ProjectType)
		assert.Equal(t, PlatformIOS, result.Platform)
		assert.False(t, result.HermesApplied)
	})

	t.Run("expo project", func(t *testing.T) {
		dir := t.TempDir()
		outputDir := filepath.Join(dir, "output")

		writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies": {"expo": "~49.0.0", "react-native": "0.72.0"}}`)
		writeFile(t, filepath.Join(dir, "index.js"), "")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			// Simulate expo export creating a bundle file
			for i, arg := range args {
				if arg == "--output-dir" && i+1 < len(args) {
					require.NoError(t, os.MkdirAll(args[i+1], 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(args[i+1], "bundle.js"), []byte("expo bundle"), 0o644))
				}
			}
		}

		opts := &BundleOptions{
			Platform:   PlatformAndroid,
			ProjectDir: dir,
			OutputDir:  outputDir,
			HermesMode: HermesModeOff,
		}

		result, err := RunWithExecutor(opts, executor, newTestLogger())
		require.NoError(t, err)

		assert.Equal(t, ProjectTypeExpo, result.ProjectType)
	})

	t.Run("react native with hermes enabled but no hermesc", func(t *testing.T) {
		dir := t.TempDir()
		outputDir := filepath.Join(dir, "output")

		writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies": {"react-native": "0.72.0"}}`)
		writeFile(t, filepath.Join(dir, "index.js"), "")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			for i, arg := range args {
				if arg == "--bundle-output" && i+1 < len(args) {
					require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o755))
					require.NoError(t, os.WriteFile(args[i+1], []byte("bundle"), 0o644))
				}
			}
		}

		opts := &BundleOptions{
			Platform:   PlatformIOS,
			ProjectDir: dir,
			OutputDir:  outputDir,
			HermesMode: HermesModeOn,
		}

		_, err := RunWithExecutor(opts, executor, newTestLogger())
		require.Error(t, err)
	})

	t.Run("invalid project directory", func(t *testing.T) {
		executor := &mockExecutor{}
		opts := &BundleOptions{
			Platform:   PlatformIOS,
			ProjectDir: "/nonexistent/path",
			OutputDir:  "/tmp/output",
			HermesMode: HermesModeOff,
		}

		_, err := RunWithExecutor(opts, executor, newTestLogger())
		require.Error(t, err)
	})

	t.Run("user overrides entry file and metro config", func(t *testing.T) {
		dir := t.TempDir()
		outputDir := filepath.Join(dir, "output")

		writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies": {"react-native": "0.72.0"}}`)
		writeFile(t, filepath.Join(dir, "index.js"), "")
		writeFile(t, filepath.Join(dir, "custom.config.js"), "")

		var capturedArgs []string
		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			capturedArgs = args
			for i, arg := range args {
				if arg == "--bundle-output" && i+1 < len(args) {
					require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o755))
					require.NoError(t, os.WriteFile(args[i+1], []byte("bundle"), 0o644))
				}
			}
		}

		opts := &BundleOptions{
			Platform:    PlatformIOS,
			ProjectDir:  dir,
			OutputDir:   outputDir,
			EntryFile:   "custom-entry.js",
			MetroConfig: filepath.Join(dir, "custom.config.js"),
			HermesMode:  HermesModeOff,
		}

		_, err := RunWithExecutor(opts, executor, newTestLogger())
		require.NoError(t, err)

		// Verify the overridden entry file was used
		foundEntry := false
		foundConfig := false
		for i, arg := range capturedArgs {
			if arg == "--entry-file" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "custom-entry.js" {
				foundEntry = true
			}
			if arg == "--config" && i+1 < len(capturedArgs) {
				foundConfig = true
			}
		}
		assert.True(t, foundEntry, "custom entry file not passed to bundler")
		assert.True(t, foundConfig, "custom metro config not passed to bundler")
	})

	t.Run("defaults output dir when empty", func(t *testing.T) {
		dir := t.TempDir()

		// An empty OutputDir resolves to the relative DefaultOutputDir ("./CodePush"), which
		// filepath.Abs resolves against the process's CWD. Chdir into the temp project dir so
		// that resolution lands there instead of polluting the package's source tree.
		t.Chdir(dir)

		writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies": {"react-native": "0.72.0"}}`)
		writeFile(t, filepath.Join(dir, "index.js"), "")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			for i, arg := range args {
				if arg == "--bundle-output" && i+1 < len(args) {
					require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o755))
					require.NoError(t, os.WriteFile(args[i+1], []byte("bundle"), 0o644))
				}
			}
		}

		opts := &BundleOptions{
			Platform:   PlatformIOS,
			ProjectDir: dir,
			OutputDir:  "",
			HermesMode: HermesModeOff,
		}

		result, err := RunWithExecutor(opts, executor, newTestLogger())
		require.NoError(t, err)

		assert.NotEmpty(t, result.OutputDir)
	})

	t.Run("defaults hermes mode when empty", func(t *testing.T) {
		dir := t.TempDir()

		// Use RN < 0.70 so auto-detection defaults to Hermes off (no hermesc needed)
		writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies": {"react-native": "0.68.0"}}`)
		writeFile(t, filepath.Join(dir, "index.js"), "")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			for i, arg := range args {
				if arg == "--bundle-output" && i+1 < len(args) {
					require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o755))
					require.NoError(t, os.WriteFile(args[i+1], []byte("bundle"), 0o644))
				}
			}
		}

		opts := &BundleOptions{
			Platform:   PlatformIOS,
			ProjectDir: dir,
			OutputDir:  filepath.Join(dir, "output"),
			HermesMode: "",
		}

		_, err := RunWithExecutor(opts, executor, newTestLogger())
		require.NoError(t, err)
	})

	t.Run("does not export bitrise summary", func(t *testing.T) {
		dir := t.TempDir()
		outputDir := filepath.Join(dir, "output")
		deployDir := filepath.Join(dir, "deploy")
		require.NoError(t, os.MkdirAll(deployDir, 0o755))

		writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies": {"react-native": "0.72.0"}}`)
		writeFile(t, filepath.Join(dir, "index.js"), "")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			for i, arg := range args {
				if arg == "--bundle-output" && i+1 < len(args) {
					require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o755))
					require.NoError(t, os.WriteFile(args[i+1], []byte("bundle"), 0o644))
				}
			}
		}

		t.Setenv("BITRISE_DEPLOY_DIR", deployDir)
		t.Setenv("BITRISE_BUILD_NUMBER", "42")

		opts := &BundleOptions{
			Platform:   PlatformIOS,
			ProjectDir: dir,
			OutputDir:  outputDir,
			HermesMode: HermesModeOff,
		}

		_, err := RunWithExecutor(opts, executor, newTestLogger())
		require.NoError(t, err)

		// RunWithExecutor no longer exports to Bitrise deploy dir; the CLI layer handles that
		summaryPath := filepath.Join(deployDir, "codepush-bundle-summary.json")
		_, err = os.Stat(summaryPath)
		assert.Error(t, err, "RunWithExecutor should not export summary; that responsibility moved to CLI layer")
	})
}

func TestCompileWithHermes(t *testing.T) {
	t.Run("skips when Hermes is disabled", func(t *testing.T) {
		executor := &mockExecutor{}
		config := &ProjectConfig{HermesEnabled: false, ProjectType: ProjectTypeReactNative}
		result := &BundleResult{}

		err := compileWithHermes(config, result, nil, executor, newTestLogger())
		require.NoError(t, err)
		assert.False(t, result.HermesApplied)
		assert.Empty(t, executor.commands)
	})

	t.Run("skips for Expo project even when Hermes enabled", func(t *testing.T) {
		executor := &mockExecutor{}
		config := &ProjectConfig{HermesEnabled: true, ProjectType: ProjectTypeExpo}
		result := &BundleResult{}

		err := compileWithHermes(config, result, nil, executor, newTestLogger())
		require.NoError(t, err)
		assert.False(t, result.HermesApplied)
		assert.Empty(t, executor.commands)
	})

	t.Run("returns error when hermesc path is empty", func(t *testing.T) {
		executor := &mockExecutor{}
		config := &ProjectConfig{
			HermesEnabled: true,
			ProjectType:   ProjectTypeReactNative,
			HermescPath:   "",
		}
		result := &BundleResult{}

		err := compileWithHermes(config, result, nil, executor, newTestLogger())
		require.Error(t, err)
		assert.ErrorContains(t, err, "hermesc was not found")
	})

	t.Run("compiles successfully with mock executor", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath := filepath.Join(dir, "index.bundle")
		hermescPath := filepath.Join(dir, "hermesc")
		writeFile(t, bundlePath, "bundle")
		writeFile(t, hermescPath, "hermesc binary")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			for i, arg := range args {
				if arg == "-out" && i+1 < len(args) {
					require.NoError(t, os.WriteFile(args[i+1], []byte("bytecode"), 0o644))
				}
			}
		}

		config := &ProjectConfig{
			HermesEnabled: true,
			ProjectType:   ProjectTypeReactNative,
			HermescPath:   hermescPath,
		}
		result := &BundleResult{BundlePath: bundlePath}

		err := compileWithHermes(config, result, nil, executor, newTestLogger())
		require.NoError(t, err)
		assert.True(t, result.HermesApplied)
		assert.Len(t, executor.commands, 1)

		// Verify the bundle file was replaced with the compiled bytecode
		data, err := os.ReadFile(bundlePath)
		require.NoError(t, err)
		assert.Equal(t, "bytecode", string(data))
	})

	t.Run("returns error when hermesc execution fails", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath := filepath.Join(dir, "index.bundle")
		hermescPath := filepath.Join(dir, "hermesc")
		writeFile(t, bundlePath, "bundle")
		writeFile(t, hermescPath, "hermesc binary")

		executor := &mockExecutor{err: &mockExitError{code: 1}}
		config := &ProjectConfig{
			HermesEnabled: true,
			ProjectType:   ProjectTypeReactNative,
			HermescPath:   hermescPath,
		}
		result := &BundleResult{BundlePath: bundlePath}

		err := compileWithHermes(config, result, nil, executor, newTestLogger())
		require.Error(t, err)
		assert.False(t, result.HermesApplied)
	})
}
