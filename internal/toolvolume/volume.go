// Package toolvolume initializes immutable shared tool volumes.
package toolvolume

import (
	"anchorage/internal/toolbundle"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// Initialize serializes writers using an atomic directory lock. Ordinary errors
// remove the lock but preserve partial contents; stale locks are never removed.
func Initialize(source, target string, output io.Writer) error {
	return initialize(source, target, output, copyBundle)
}
func initialize(source, target string, output io.Writer, copyFiles func(*os.Root, *os.Root) error) (err error) {
	dst, err := os.OpenRoot(target)
	if err != nil {
		return fmt.Errorf("mount a named volume at %s: %w", target, err)
	}
	defer dst.Close()
	if err := dst.Mkdir(".initializing", 0755); err != nil {
		return fmt.Errorf("volume initialization already running or previously failed: %w", err)
	}
	defer func() {
		if cleanup := dst.Remove(".initializing"); err == nil {
			err = cleanup
		}
	}()
	src, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer src.Close()
	ready, err := dst.Lstat(".ready")
	if err == nil {
		if !ready.Mode().IsRegular() {
			return fmt.Errorf("invalid ready marker")
		}
		for _, name := range []string{"manifest.json", "SHA256SUMS"} {
			a, err := readFile(src, name)
			if err != nil {
				return err
			}
			b, err := readFile(dst, name)
			if err != nil {
				return err
			}
			if !bytes.Equal(a, b) {
				return fmt.Errorf("volume %s differs from bundle", name)
			}
		}
		return verify(dst)
	}
	if !os.IsNotExist(err) {
		return err
	}
	dir, err := dst.Open(".")
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(-1)
	closeErr := dir.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) != 1 || entries[0].Name() != ".initializing" {
		return fmt.Errorf("refusing to initialize a nonempty volume")
	}
	if err := copyFiles(src, dst); err != nil {
		return err
	}
	if err := verify(dst); err != nil {
		return err
	}
	if err := toolbundle.WriteFile(dst, ".ready", bytes.NewReader(nil), 0644); err != nil {
		return err
	}
	fmt.Fprintln(output, "Tool volume initialized")
	return nil
}
func readFile(root *os.Root, name string) ([]byte, error) {
	f, err := toolbundle.OpenFile(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
func copyBundle(src, dst *os.Root) error {
	return fs.WalkDir(src.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if name == ".ready" || name == ".initializing" {
			return fmt.Errorf("reserved bundle file %q", name)
		}
		if entry.IsDir() {
			return toolbundle.MakeDirectory(dst, name)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported bundle file %q", name)
		}
		f, err := toolbundle.OpenFile(src, name)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if info.Mode()&0111 != 0 {
			mode = 0755
		}
		return toolbundle.WriteFile(dst, name, f, mode)
	})
}
func verify(root *os.Root) error {
	sums, err := toolbundle.OpenFile(root, "SHA256SUMS")
	if err != nil {
		return err
	}
	defer sums.Close()
	scanner := bufio.NewScanner(sums)
	previous := ""
	hasManifest := false
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return fmt.Errorf("malformed SHA256SUMS line")
		}
		expected, err := hex.DecodeString(line[:64])
		if err != nil {
			return fmt.Errorf("invalid SHA256SUMS hash: %w", err)
		}
		name := line[66:]
		if name <= previous || name == "SHA256SUMS" || strings.HasPrefix(name, ".") {
			return fmt.Errorf("invalid or unsorted SHA256SUMS path %q", name)
		}
		previous = name
		hasManifest = hasManifest || name == "manifest.json"
		f, err := toolbundle.OpenFile(root, name)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if !bytes.Equal(h.Sum(nil), expected) {
			return fmt.Errorf("checksum mismatch: %s", name)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !hasManifest {
		return fmt.Errorf("SHA256SUMS must include manifest.json")
	}
	return nil
}
