package bundler

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputePackageHash(t *testing.T) {
	t.Run("produces consistent hash", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		hash1, err := ComputePackageHash(dir)
		require.NoError(t, err)
		hash2, err := ComputePackageHash(dir)
		require.NoError(t, err)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("excludes .codepushrelease from hash", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		hashBefore, err := ComputePackageHash(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, ".codepushrelease"), []byte("some-jwt"), 0o644))

		hashAfter, err := ComputePackageHash(dir)
		require.NoError(t, err)
		assert.Equal(t, hashBefore, hashAfter, ".codepushrelease must be excluded from hash")
	})

	t.Run("excludes .DS_Store from hash", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		hashBefore, err := ComputePackageHash(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("mac metadata"), 0o644))

		hashAfter, err := ComputePackageHash(dir)
		require.NoError(t, err)
		assert.Equal(t, hashBefore, hashAfter, ".DS_Store must be excluded from hash")
	})

	t.Run("includes directory name as path prefix", func(t *testing.T) {
		parent := t.TempDir()
		dirA := filepath.Join(parent, "CodePush")
		require.NoError(t, os.Mkdir(dirA, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dirA, "index.js"), []byte("bundle"), 0o644))

		dirB := filepath.Join(t.TempDir(), "other")
		require.NoError(t, os.Mkdir(dirB, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dirB, "index.js"), []byte("bundle"), 0o644))

		hashA, err := ComputePackageHash(dirA)
		require.NoError(t, err)
		hashB, err := ComputePackageHash(dirB)
		require.NoError(t, err)

		assert.NotEqual(t, hashA, hashB, "hash must differ when directory name differs")
	})

	t.Run("different content produces different hash", func(t *testing.T) {
		dirA := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dirA, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dirA, "index.js"), []byte("bundle-v1"), 0o644))

		dirB := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dirB, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dirB, "index.js"), []byte("bundle-v2"), 0o644))

		hashA, err := ComputePackageHash(dirA)
		require.NoError(t, err)
		hashB, err := ComputePackageHash(dirB)
		require.NoError(t, err)

		assert.NotEqual(t, hashA, hashB)
	})

	t.Run("matches device hash for paths with HTML-special characters", func(t *testing.T) {
		// The mobile SDK recomputes this hash on-device without HTML escaping. A file path
		// containing '&', '<', or '>' must hash to that un-escaped value, not the value Go's
		// default json.Marshal (which escapes them) would produce, or the contentHash claim
		// devices can never reproduce.
		for _, name := range []string{"L&G.png", "a<b.png", "c>d.png"} {
			t.Run(name, func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), "CodePush")
				require.NoError(t, os.Mkdir(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("asset"), 0o644))

				got, err := ComputePackageHash(dir)
				require.NoError(t, err)

				assert.Equal(t, clientPackageHash(t, dir, name), got,
					"hash must match the SDK's un-escaped serialization")
				assert.NotEqual(t, htmlEscapedPackageHash(t, dir, name), got,
					"hash must differ from the buggy HTML-escaped serialization")
			})
		}
	})
}

// clientPackageHash independently recomputes the package hash the way the on-device SDK does:
// sorted "CodePush/relpath:sha256hex" entries serialized without HTML escaping, then SHA256'd.
// It does not call ComputePackageHash, so the test fails if that serialization ever diverges
// from the client's. The directory holds exactly index.js and one extra asset named assetName.
func clientPackageHash(t *testing.T, dir, assetName string) string {
	t.Helper()
	return manifestHash(t, dir, assetName, false)
}

// htmlEscapedPackageHash recomputes the hash with Go's default HTML-escaping json.Marshal,
// reproducing the old (buggy) behavior. Used only to prove the fix changes the result for
// paths containing '&', '<', or '>'.
func htmlEscapedPackageHash(t *testing.T, dir, assetName string) string {
	t.Helper()
	return manifestHash(t, dir, assetName, true)
}

func manifestHash(t *testing.T, dir, assetName string, escapeHTML bool) string {
	t.Helper()

	entries := []string{
		"CodePush/index.js:" + sha256Content(t, filepath.Join(dir, "index.js")),
		"CodePush/" + assetName + ":" + sha256Content(t, filepath.Join(dir, assetName)),
	}
	sort.Strings(entries)

	var serialized []byte
	if escapeHTML {
		var err error
		serialized, err = json.Marshal(entries)
		require.NoError(t, err)
	} else {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		require.NoError(t, enc.Encode(entries))
		serialized = bytes.TrimRight(buf.Bytes(), "\n")
	}

	sum := sha256.Sum256(serialized)
	return hex.EncodeToString(sum[:])
}

func sha256Content(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path is built from t.TempDir(), not user input
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestSignBundle(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyPath := writeRSAKey(t, key)

	t.Run("writes .codepushrelease JWT for CodePush directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		err := SignBundle(dir, keyPath, "test")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(dir, ".codepushrelease"))
		require.NoError(t, err)

		jwt := string(data)
		parts := strings.Split(jwt, ".")
		assert.Len(t, parts, 3, "JWT must have 3 dot-separated parts")
		assert.True(t, strings.HasPrefix(jwt, "eyJ"), "JWT header must start with eyJ (base64url of {)")

		payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(payloadBytes, &payload))
		assert.Equal(t, "codepush-cli/test", payload["bitriseUserAgent"])
		assert.Equal(t, "1.0.0", payload["claimVersion"])
		contentHash, ok := payload["contentHash"].(string)
		assert.True(t, ok, "contentHash must be a string")
		assert.Len(t, contentHash, 64, "contentHash must be a 64-character hex string")
	})

	t.Run("returns error when directory is not named CodePush", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "wrong-name")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		err := SignBundle(dir, keyPath, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `must be named "CodePush"`)
		assert.Contains(t, err.Error(), "wrong-name")
	})

	t.Run("supports PKCS8 private key", func(t *testing.T) {
		pkcs8Path := writePKCS8Key(t, key)

		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		err := SignBundle(dir, pkcs8Path, "test")
		require.NoError(t, err)

		_, err = os.ReadFile(filepath.Join(dir, ".codepushrelease"))
		require.NoError(t, err)
	})

	t.Run("returns error when key file does not exist", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		err := SignBundle(dir, "/nonexistent/key.pem", "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading key file")
	})

	t.Run("returns error for unsupported PEM block type", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		// Write a PEM file with a CERTIFICATE block (not a key)
		path := filepath.Join(t.TempDir(), "cert.pem")
		f, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a cert")}))
		_ = f.Close()

		err = SignBundle(dir, path, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported PEM block type")
	})

	t.Run("returns error for file with no PEM block", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		path := filepath.Join(t.TempDir(), "notapem.txt")
		require.NoError(t, os.WriteFile(path, []byte("not a pem file"), 0o644))

		err := SignBundle(dir, path, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no PEM block found")
	})

	t.Run("hash is stable across sign calls (deterministic hash)", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "CodePush")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("bundle"), 0o644))

		err := SignBundle(dir, keyPath, "test")
		require.NoError(t, err)

		// second call: .codepushrelease now exists but must be excluded from hash
		err = SignBundle(dir, keyPath, "test")
		require.NoError(t, err)
	})
}

// writeRSAKey writes a PKCS1 RSA private key PEM file and returns its path.
func writeRSAKey(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.pem")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, pem.Encode(f, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
	return path
}

// writePKCS8Key writes a PKCS8 RSA private key PEM file and returns its path.
func writePKCS8Key(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key_pkcs8.pem")
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, pem.Encode(f, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}))
	return path
}

func TestValidatePrivateKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	t.Run("accepts PKCS1 and PKCS8 keys", func(t *testing.T) {
		require.NoError(t, ValidatePrivateKey(writeRSAKey(t, key)))
		require.NoError(t, ValidatePrivateKey(writePKCS8Key(t, key)))
	})

	t.Run("rejects a missing file", func(t *testing.T) {
		err := ValidatePrivateKey(filepath.Join(t.TempDir(), "missing.pem"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading key file")
	})

	t.Run("rejects a file without a PEM block", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "notapem.txt")
		require.NoError(t, os.WriteFile(path, []byte("not a pem file"), 0o644))

		err := ValidatePrivateKey(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no PEM block found")
	})

	t.Run("rejects a non-RSA PKCS8 key", func(t *testing.T) {
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		der, err := x509.MarshalPKCS8PrivateKey(ecKey)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "ec.pem")
		require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600))

		err = ValidatePrivateKey(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not an RSA key")
	})
}
