package toolbundle

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"
)

// PrepareSQLiteSource verifies the official amalgamation before opening its ZIP.
// Sources are written to a build-only directory, separate from the final bundle.
func PrepareSQLiteSource(ctx context.Context, client *http.Client, spec Spec, destination string) error {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	return install(ctx, client, root, "sqlite3", spec)
}

func extractSQLiteSource(root *os.Root, source io.ReaderAt, size int64) error {
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return err
	}
	required := map[string]bool{"sqlite3.c": false, "shell.c": false, "sqlite3.h": false, "sqlite3ext.h": false}
	seen := make(map[string]bool)
	for _, entry := range archive.File {
		if err := ValidPath(entry.Name); err != nil {
			return err
		}
		name := path.Clean(entry.Name)
		if seen[name] {
			return fmt.Errorf("duplicate SQLite archive path %q", name)
		}
		seen[name] = true
		if entry.Mode().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() {
			return fmt.Errorf("unsupported SQLite archive entry %q", name)
		}
		base := path.Base(name)
		installed, selected := required[base]
		if !selected {
			continue
		}
		if installed {
			return fmt.Errorf("duplicate SQLite source %q", base)
		}
		f, err := entry.Open()
		if err != nil {
			return err
		}
		writeErr := WriteFile(root, base, f, 0644)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		required[base] = true
	}
	for name, installed := range required {
		if !installed {
			return fmt.Errorf("missing SQLite source %q", name)
		}
	}
	return nil
}
