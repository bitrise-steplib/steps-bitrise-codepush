package step

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRequiredInputs(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("project_dir", dir)
	t.Setenv("BITRISE_DEPLOY_DIR", dir)
	t.Setenv("platform", "android")
	t.Setenv("app_id", "app")
	t.Setenv("deployment", "Staging")
	t.Setenv("api_token", "token")
	t.Setenv("app_version", "1.0.0")
	t.Setenv("hermes", "auto")
	t.Setenv("skip_dependency_install", "false")
	t.Setenv("mandatory", "false")
	t.Setenv("disabled_after_upload", "false")
	t.Setenv("verbose_log", "false")
}

func newTestStep() Step {
	return New(stepconf.NewInputParser(env.NewRepository()), log.NewLogger(), export.NewExporter(nil, export.NewFileManager()))
}

func TestProcessConfigPrivateKey(t *testing.T) {
	t.Run("unsigned when private_key_path is empty", func(t *testing.T) {
		setRequiredInputs(t)
		t.Setenv("private_key_path", "")

		cfg, err := newTestStep().ProcessConfig()
		require.NoError(t, err)
		assert.Empty(t, cfg.PrivateKeyPath)
	})

	t.Run("accepts a valid key", func(t *testing.T) {
		setRequiredInputs(t)
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "key.pem")
		block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
		require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
		t.Setenv("private_key_path", path)

		cfg, err := newTestStep().ProcessConfig()
		require.NoError(t, err)
		assert.Equal(t, path, cfg.PrivateKeyPath)
	})

	t.Run("fails fast on a missing key file", func(t *testing.T) {
		setRequiredInputs(t)
		t.Setenv("private_key_path", filepath.Join(t.TempDir(), "missing.pem"))

		_, err := newTestStep().ProcessConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid private_key_path")
	})
}
