package zip

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectory(t *testing.T) {
	t.Run("zips files correctly", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "bundle")
		os.Mkdir(srcDir, 0o755)

		writeFile(t, filepath.Join(srcDir, "main.jsbundle"), "bundle content")
		writeFile(t, filepath.Join(srcDir, "main.jsbundle.map"), "sourcemap content")

		zipPath, err := Directory(srcDir)
		require.NoError(t, err)
		defer os.Remove(zipPath)

		assert.Equal(t, srcDir+".zip", zipPath)

		entries := readZipEntries(t, zipPath)
		sort.Strings(entries)

		// The source directory name ("bundle") becomes the top-level entry.
		assert.Equal(t, []string{"bundle/", "bundle/main.jsbundle", "bundle/main.jsbundle.map"}, entries)
	})

	t.Run("preserves nested directory structure", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "bundle")
		os.MkdirAll(filepath.Join(srcDir, "assets", "images"), 0o755)

		writeFile(t, filepath.Join(srcDir, "index.js"), "code")
		writeFile(t, filepath.Join(srcDir, "assets", "images", "logo.png"), "image data")

		zipPath, err := Directory(srcDir)
		require.NoError(t, err)
		defer os.Remove(zipPath)

		entries := readZipEntries(t, zipPath)
		sort.Strings(entries)

		expected := []string{"bundle/", "bundle/assets/", "bundle/assets/images/", "bundle/assets/images/logo.png", "bundle/index.js"}
		assert.Equal(t, expected, entries)
	})

	t.Run("preserves file contents", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "bundle")
		os.Mkdir(srcDir, 0o755)

		content := "console.log('hello world')"
		writeFile(t, filepath.Join(srcDir, "app.js"), content)

		zipPath, err := Directory(srcDir)
		require.NoError(t, err)
		defer os.Remove(zipPath)

		r, err := zip.OpenReader(zipPath)
		require.NoError(t, err)
		defer r.Close()

		for _, f := range r.File {
			if f.Name == "bundle/app.js" {
				rc, err := f.Open()
				require.NoError(t, err)
				defer rc.Close()

				buf := make([]byte, len(content)+10)
				n, _ := rc.Read(buf)
				assert.Equal(t, content, string(buf[:n]))
				return
			}
		}
		t.Error("app.js not found in zip")
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := Directory("/nonexistent/path")
		require.Error(t, err)
	})

	t.Run("source is a file not directory", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "notadir")
		writeFile(t, filePath, "content")

		_, err := Directory(filePath)
		require.Error(t, err)
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "empty")
		os.Mkdir(srcDir, 0o755)

		zipPath, err := Directory(srcDir)
		require.NoError(t, err)
		defer os.Remove(zipPath)

		entries := readZipEntries(t, zipPath)
		// The (empty) source directory itself is still recorded as a top-level entry.
		assert.Equal(t, []string{"empty/"}, entries)
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readZipEntries(t *testing.T, zipPath string) []string {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer r.Close()

	var entries []string
	for _, f := range r.File {
		entries = append(entries, f.Name)
	}
	return entries
}
