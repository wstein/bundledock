// Package toolbundle builds checksum-pinned, native OCI tool bundles.
package toolbundle

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// ValidPath accepts only relative slash-separated paths without parent traversal.
func ValidPath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("unsafe bundle path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("unsafe bundle path %q", name)
		}
	}
	return nil
}

// CheckPath rejects symbolic links in any existing component, including the leaf.
func CheckPath(root *os.Root, name string) error {
	if err := ValidPath(name); err != nil {
		return err
	}
	parts := strings.Split(path.Clean(name), "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link in bundle path %q", name)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("non-directory in bundle path %q", name)
		}
	}
	return nil
}

func mkdir(root *os.Root, name string) error {
	if name == "." {
		return nil
	}
	if err := CheckPath(root, name); err != nil {
		return err
	}
	parts := strings.Split(path.Clean(name), "/")
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		if err := root.Mkdir(p, 0755); err != nil && !os.IsExist(err) {
			return err
		}
		dir, err := root.Open(p)
		if err != nil {
			return err
		}
		info, err := dir.Stat()
		if err == nil && !info.IsDir() {
			err = fmt.Errorf("not a directory: %s", p)
		}
		if err == nil {
			err = dir.Chmod(0755)
		}
		closeErr := dir.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// WriteFile creates a new regular file and normalizes its permissions.
func WriteFile(root *os.Root, name string, source io.Reader, mode fs.FileMode) error {
	if err := CheckPath(root, name); err != nil {
		return err
	}
	if err := mkdir(root, path.Dir(name)); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, source)
	if err == nil {
		err = f.Chmod(mode)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// OpenFile opens a regular file without accepting symbolic-link components.
func OpenFile(root *os.Root, name string) (*os.File, error) {
	if err := CheckPath(root, name); err != nil {
		return nil, err
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("not a regular bundle file: %s", name)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// MakeDirectory creates a confined directory with normalized permissions.
func MakeDirectory(root *os.Root, name string) error { return mkdir(root, name) }
