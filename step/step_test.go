// Package step tests for Step.ProcessConfig(). These are new tests written for this port (the CLI
// this step's logic is ported from has no equivalent step-input-parsing layer); they exercise
// stepconf.NewInputParser against an in-memory env.Repository fake, and a no-op log.Logger fake
// satisfying the full Logger interface.
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
		"app_id":                  "app-123",
		"deployment":              "Production",
		"app_version":             "1.0.0",
		"description":             "",
		"rollout_percentage":      "",
		"mandatory":               "false",
		"disabled_after_upload":   "false",
		"api_token":               "test-token",
		"server_url":              "",
		"BITRISE_DEPLOY_DIR":      deployDir,
		"verbose_log":             "false",
	}
}

func newTestStep(env map[string]string) Step {
	repo := newFakeEnvRepository(env)
	parser := stepconf.NewInputParser(repo)
	return New(parser, &fakeLogger{}, export.Exporter{})
}

func TestProcessConfig_Defaults(t *testing.T) {
	env := validEnv(t)
	// rollout_percentage and server_url intentionally left empty in validEnv to exercise defaults.

	s := newTestStep(env)
	cfg, err := s.ProcessConfig()
	require.NoError(t, err)

	assert.Equal(t, "100", cfg.RolloutPercentage, "empty rollout_percentage should default to 100")
	assert.Equal(t, "https://api.bitrise.io", cfg.ServerURL, "empty server_url should default to the Bitrise API")
}

func TestProcessConfig_ServerURLTrailingSlashTrimmed(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"default has no trailing slash", "", "https://api.bitrise.io"},
		{"single trailing slash trimmed", "https://api.bitrise.io/", "https://api.bitrise.io"},
		{"multiple trailing slashes trimmed", "https://custom.example.com///", "https://custom.example.com"},
		{"custom URL without trailing slash unchanged", "https://custom.example.com", "https://custom.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv(t)
			env["server_url"] = tt.input

			s := newTestStep(env)
			cfg, err := s.ProcessConfig()
			require.NoError(t, err)

			assert.Equal(t, tt.want, cfg.ServerURL)
		})
	}
}

func TestProcessConfig_RolloutPercentageOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"above 100", "150"},
		{"negative", "-5"},
		{"non-numeric", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv(t)
			env["rollout_percentage"] = tt.value

			s := newTestStep(env)
			_, err := s.ProcessConfig()
			require.Error(t, err)
			assert.ErrorContains(t, err, "rollout_percentage")
		})
	}
}

func TestProcessConfig_RolloutPercentageValidBoundaries(t *testing.T) {
	tests := []string{"0", "50", "100"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			env := validEnv(t)
			env["rollout_percentage"] = value

			s := newTestStep(env)
			cfg, err := s.ProcessConfig()
			require.NoError(t, err)
			assert.Equal(t, value, cfg.RolloutPercentage)
		})
	}
}

func TestProcessConfig_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"missing app_id", "app_id"},
		{"missing deployment", "deployment"},
		{"missing app_version", "app_version"},
		{"missing api_token", "api_token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv(t)
			env[tt.key] = ""

			s := newTestStep(env)
			_, err := s.ProcessConfig()
			require.Error(t, err)
		})
	}
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

func TestProcessConfig_ValidConfigPopulatesFields(t *testing.T) {
	env := validEnv(t)
	env["app_id"] = "my-app"
	env["deployment"] = "Staging"
	env["app_version"] = "2.3.4"
	env["rollout_percentage"] = "42"

	s := newTestStep(env)
	cfg, err := s.ProcessConfig()
	require.NoError(t, err)

	assert.Equal(t, "my-app", cfg.AppID)
	assert.Equal(t, "Staging", cfg.Deployment)
	assert.Equal(t, "2.3.4", cfg.AppVersion)
	assert.Equal(t, "42", cfg.RolloutPercentage)
	assert.Equal(t, stepconf.Secret("test-token"), cfg.APIToken)
}
