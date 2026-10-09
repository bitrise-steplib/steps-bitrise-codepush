package bundler_test

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bitrise-steplib/steps-bitrise-codepush/internal/bundler"
	ziputil "github.com/bitrise-steplib/steps-bitrise-codepush/internal/zip"
)

// TestSignedBundleSurvivesZipping signs a bundle, packages it with the real zip code and then
// checks the archive the way the on-device SDK does: .codepushrelease must be inside CodePush/,
// its JWT must verify with the public key, and its contentHash must equal a hash recomputed
// from the archive's own contents. The hash is recomputed here independently of the signer.
func TestSignedBundleSurvivesZipping(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyPath := writePKCS1Key(t, key)

	dir := filepath.Join(t.TempDir(), "CodePush")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.android.bundle"), []byte("bundle"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "logo&mark.png"), []byte("png"), 0o644))

	require.NoError(t, bundler.SignBundleForStep(dir, keyPath))

	zipPath, err := ziputil.Directory(dir)
	require.NoError(t, err)

	zr, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = zr.Close() })

	var entries []string
	var jwt string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		data := readZipFile(t, f)
		if f.Name == "CodePush/.codepushrelease" {
			jwt = string(data)
			continue
		}
		sum := sha256.Sum256(data)
		entries = append(entries, f.Name+":"+hex.EncodeToString(sum[:]))
	}
	require.NotEmpty(t, jwt, "archive must contain CodePush/.codepushrelease")

	sort.Strings(entries)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(entries))
	wantHash := sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n"))

	parts := strings.Split(jwt, ".")
	require.Len(t, parts, 3)

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	signed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	require.NoError(t, rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, signed[:], sig), "JWT signature must verify")

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"alg":"RS256","typ":"JWT"}`, string(headerJSON))

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	assert.Equal(t, hex.EncodeToString(wantHash[:]), payload["contentHash"])
	assert.Equal(t, "1.0.0", payload["claimVersion"])
	assert.Equal(t, "codepush-cli/1.3.1", payload["bitriseUserAgent"])
}

func readZipFile(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	return data
}

func writePKCS1Key(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.pem")
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
	return path
}
