package zip

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func addFileToZip(w *zip.Writer, baseDir string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}

		if relPath == "." {
			return nil
		}

		// Zip spec requires forward slashes
		zipEntryName := filepath.ToSlash(relPath)

		if info.IsDir() {
			_, err := w.Create(zipEntryName + "/")
			return err
		}

		writer, err := w.Create(zipEntryName)
		if err != nil {
			return fmt.Errorf("creating zip entry %s: %w", zipEntryName, err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening file %s: %w", path, err)
		}
		defer func() { _ = file.Close() }()

		_, err = io.Copy(writer, file)
		return err
	}
}

func Directory(srcDir string) (string, error) {
	absDir, err := filepath.Abs(srcDir)
	if err != nil {
		return "", fmt.Errorf("resolving directory path: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return "", fmt.Errorf("source directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source path is not a directory: %s", absDir)
	}

	zipPath := absDir + ".zip"
	f, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("creating zip file: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := zip.NewWriter(f)
	defer func() { _ = w.Close() }()

	// Walk relative to the parent so the source directory's own name becomes the
	// top-level entry in the archive (e.g. "CodePush/index.android.bundle"). The
	// react-native-code-push SDK extracts the package and expects the bundle under
	// that named subdirectory, and computes the content hash with the same prefix.
	parentDir := filepath.Dir(absDir)
	err = filepath.Walk(absDir, addFileToZip(w, parentDir))
	if err != nil {
		return "", fmt.Errorf("adding files to zip: %w", err)
	}

	return zipPath, nil
}
