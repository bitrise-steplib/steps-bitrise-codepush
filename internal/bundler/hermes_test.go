package bundler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHermesCompilerCompile(t *testing.T) {
	t.Run("successful compilation", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath := filepath.Join(dir, "main.jsbundle")
		hermescPath := filepath.Join(dir, "hermesc")

		writeFile(t, bundlePath, "console.log('hello')")
		writeFile(t, hermescPath, "")

		executor := &mockExecutor{}
		// Simulate hermesc creating the .hbc file
		executor.onRun = func(_ string, _ string, args ...string) {
			for i, arg := range args {
				if arg == "-out" && i+1 < len(args) {
					require.NoError(t, os.WriteFile(args[i+1], []byte("bytecode"), 0o644))
				}
			}
		}

		compiler := NewHermesCompiler(executor, newTestLogger())
		err := compiler.Compile(hermescPath, bundlePath, nil)
		require.NoError(t, err)

		// Verify the command was called correctly
		require.Len(t, executor.commands, 1)

		cmd := executor.commands[0]
		assert.Equal(t, hermescPath, cmd.name)

		// Check args include -emit-binary
		assert.Contains(t, cmd.args, "-emit-binary")

		// Verify the .hbc file was renamed to the original bundle path
		data, err := os.ReadFile(bundlePath)
		require.NoError(t, err)
		assert.Equal(t, "bytecode", string(data))
	})

	t.Run("extra hermes flags are passed before the input file", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath := filepath.Join(dir, "main.jsbundle")
		hermescPath := filepath.Join(dir, "hermesc")

		writeFile(t, bundlePath, "console.log('hello')")
		writeFile(t, hermescPath, "")

		executor := &mockExecutor{}
		executor.onRun = func(_ string, _ string, args ...string) {
			for i, arg := range args {
				if arg == "-out" && i+1 < len(args) {
					require.NoError(t, os.WriteFile(args[i+1], []byte("bytecode"), 0o644))
				}
			}
		}

		compiler := NewHermesCompiler(executor, newTestLogger())
		err := compiler.Compile(hermescPath, bundlePath, []string{"-O", "-w"})
		require.NoError(t, err)

		cmd := executor.commands[0]
		// Extra flags must appear before the input file
		inputIdx := -1
		oIdx := -1
		wIdx := -1
		for i, arg := range cmd.args {
			switch arg {
			case bundlePath:
				inputIdx = i
			case "-O":
				oIdx = i
			case "-w":
				wIdx = i
			}
		}
		require.NotEqual(t, -1, oIdx, "-O flag missing")
		require.NotEqual(t, -1, wIdx, "-w flag missing")
		assert.Less(t, oIdx, inputIdx, "-O must come before input file")
		assert.Less(t, wIdx, inputIdx, "-w must come before input file")
	})

	t.Run("hermesc binary not found", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath := filepath.Join(dir, "main.jsbundle")
		writeFile(t, bundlePath, "console.log('hello')")

		executor := &mockExecutor{}
		compiler := NewHermesCompiler(executor, newTestLogger())

		err := compiler.Compile("/nonexistent/hermesc", bundlePath, nil)
		require.Error(t, err)
	})

	t.Run("bundle file not found", func(t *testing.T) {
		dir := t.TempDir()
		hermescPath := filepath.Join(dir, "hermesc")
		writeFile(t, hermescPath, "")

		executor := &mockExecutor{}
		compiler := NewHermesCompiler(executor, newTestLogger())

		err := compiler.Compile(hermescPath, "/nonexistent/bundle.js", nil)
		require.Error(t, err)
	})

	t.Run("hermesc execution fails", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath := filepath.Join(dir, "main.jsbundle")
		hermescPath := filepath.Join(dir, "hermesc")

		writeFile(t, bundlePath, "console.log('hello')")
		writeFile(t, hermescPath, "")

		executor := &mockExecutor{err: &mockExitError{code: 1}}
		compiler := NewHermesCompiler(executor, newTestLogger())

		err := compiler.Compile(hermescPath, bundlePath, nil)
		require.Error(t, err)
	})
}
