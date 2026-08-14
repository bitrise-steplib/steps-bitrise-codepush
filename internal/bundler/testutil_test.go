package bundler

import "io"

// testLogger is a no-op Logger test double. The original CLI tests used
// output.NewTest(io.Discard); this step's bundler package uses the local
// Logger interface instead (see bundler.go), so tests get a trivial stand-in
// rather than reusing the CLI's internal/output package.
type testLogger struct{}

func newTestLogger() *testLogger { return &testLogger{} }

func (l *testLogger) Infof(format string, args ...interface{})  {}
func (l *testLogger) Debugf(format string, args ...interface{}) {}
func (l *testLogger) Warnf(format string, args ...interface{})  {}
func (l *testLogger) Errorf(format string, args ...interface{}) {}

// mockExecutor records commands instead of executing them.
type mockExecutor struct {
	commands []executedCommand
	err      error
	// onRun is called during Run, allowing tests to create output files.
	onRun func(dir string, name string, args ...string)
}

type executedCommand struct {
	dir  string
	name string
	args []string
}

func (m *mockExecutor) Run(dir string, _ io.Writer, _ io.Writer, name string, args ...string) error {
	m.commands = append(m.commands, executedCommand{dir: dir, name: name, args: args})
	if m.onRun != nil {
		m.onRun(dir, name, args...)
	}
	return m.err
}

// mockExitError simulates a process exit error.
type mockExitError struct {
	code int
}

func (e *mockExitError) Error() string {
	return "exit status 1"
}
