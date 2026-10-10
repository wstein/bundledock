package toolbundle

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
)

type zipTarget struct {
	name string
	mode fs.FileMode
}

// extractSelectedZIP validates all members, including unselected files, and
// installs exactly one regular file for every selected basename.
func extractSelectedZIP(root *os.Root, source io.ReaderAt, size int64, selected map[string]zipTarget) error {
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	installed := make(map[string]bool)
	for _, entry := range archive.File {
		if err := ValidPath(entry.Name); err != nil {
			return err
		}
		name := path.Clean(entry.Name)
		if seen[name] {
			return fmt.Errorf("duplicate ZIP archive path %q", name)
		}
		seen[name] = true
		if entry.Mode().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() {
			return fmt.Errorf("unsupported ZIP archive entry %q", name)
		}
		base := path.Base(name)
		target, ok := selected[base]
		if !ok {
			continue
		}
		if installed[base] {
			return fmt.Errorf("duplicate ZIP selection %q", base)
		}
		if target.mode&0111 != 0 && entry.Mode().Perm()&0111 == 0 {
			return fmt.Errorf("ZIP executable %q is not executable", base)
		}
		f, err := entry.Open()
		if err != nil {
			return err
		}
		writeErr := WriteFile(root, target.name, f, target.mode)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		installed[base] = true
	}
	for name := range selected {
		if !installed[name] {
			return fmt.Errorf("missing ZIP selection %q", name)
		}
	}
	return nil
}
