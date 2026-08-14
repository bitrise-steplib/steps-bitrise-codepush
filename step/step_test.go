// Package step tests for Step.ProcessConfig(). These exercise stepconf.NewInputParser against an
// in-memory env.Repository fake, and a no-op log.Logger fake satisfying the full Logger interface.
package step

import (
	"path/filepath"
	"testing"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEnvRepository is a minimal in-memory env.Repository test double.
type fakeEnvRepository struct {
	values map[string]string
}

func newFakeEnvRepository(values map[string]string) *fakeEnvRepository {
	m := make(map[string]string, len(values))
	for k, v := range values {
		m[k] = v
	}
	return &fakeEnvRepository{values: m}
}

func (f *fakeEnvRepository) List() []string {
	out := make([]string, 0, len(f.values))
	for k, v := range f.values {
		out = append(out, k+"="+v)
	}
	return out
}

func (f *fakeEnvRepository) Unset(key string) error {
	delete(f.values, key)
	return nil
}

func (f *fakeEnvRepository) Get(key string) string {
	return f.values[key]
}

func (f *fakeEnvRepository) Set(key, value string) error {
	f.values[key] = value
	return nil
}

// fakeLogger is a no-op log.Logger test double.
type fakeLogger struct{}

func (l *fakeLogger) Infof(format string, v ...interface{})   {}
func (l *fakeLogger) Warnf(format string, v ...interface{})   {}
func (l *fakeLogger) Printf(format string, v ...interface{})  {}
func (l *fakeLogger) Donef(format string, v ...interface{})   {}
func (l *fakeLogger) Debugf(format string, v ...interface{})  {}
func (l *fakeLogger) Errorf(format string, v ...interface{})  {}
func (l *fakeLogger) TInfof(format string, v ...interface{})  {}
func (l *fakeLogger) TWarnf(format string, v ...interface{})  {}
func (l *fakeLogger) TPrintf(format string, v ...interface{}) {}
func (l *fakeLogger) TDonef(format string, v ...interface{})  {}
func (l *fakeLogger) TDebugf(format string, v ...interface{}) {}
func (l *fakeLogger) TErrorf(format string, v ...interface{}) {}
func (l *fakeLogger) Println()                                {}
func (l *fakeLogger) EnableDebugLog(enable bool)              {}

// validEnv returns a complete set of env vars that satisfies every input constraint declared on
// Config (required fields, opt[...] enums, and dir-must-exist checks), so individual tests can
// override just the keys they care about.
func validEnv(t *testing.T) map[string]string {
	t.Helper()
	projectDir := t.TempDir()
	deployDir := t.TempDir()

	return map[string]string{
		"project_dir":             projectDir,
		"platform":                "ios",
		"entry_file":              "",
		"bundle_name":             "",
		"hermes":                  "auto",
		"skip_dependency_install": "false",
		"BITRISE_DEPLOY_DIR":      deployDir,
		"verbose_log":             "false",
	}
}

func newTestStep(env map[string]string) Step {
	repo := newFakeEnvRepository(env)
	parser := stepconf.NewInputParser(repo)
	return New(parser, &fakeLogger{}, export.Exporter{})
}

func TestProcessConfig_InvalidPlatform(t *testing.T) {
	env := validEnv(t)
	env["platform"] = "windows"

	s := newTestStep(env)
	_, err := s.ProcessConfig()
	require.Error(t, err)
}

func TestProcessConfig_InvalidHermesMode(t *testing.T) {
	env := validEnv(t)
	env["hermes"] = "sometimes"

	s := newTestStep(env)
	_, err := s.ProcessConfig()
	require.Error(t, err)
}

func TestProcessConfig_ProjectDirMustExist(t *testing.T) {
	env := validEnv(t)
	env["project_dir"] = filepath.Join(t.TempDir(), "does-not-exist")

	s := newTestStep(env)
	_, err := s.ProcessConfig()
	require.Error(t, err)
}

func TestProcessConfig_DeployDirMustExist(t *testing.T) {
	env := validEnv(t)
	env["BITRISE_DEPLOY_DIR"] = filepath.Join(t.TempDir(), "does-not-exist")

	s := newTestStep(env)
	_, err := s.ProcessConfig()
	require.Error(t, err)
}

func TestProcessConfig_ValidConfigPopulatesFields(t *testing.T) {
	env := validEnv(t)
	env["platform"] = "android"
	env["bundle_name"] = "custom.bundle"

	s := newTestStep(env)
	cfg, err := s.ProcessConfig()
	require.NoError(t, err)

	assert.Equal(t, "android", cfg.Platform)
	assert.Equal(t, "custom.bundle", cfg.BundleName)
	assert.Equal(t, "auto", cfg.HermesMode)
	assert.False(t, cfg.SkipDependencyInstall)
}
