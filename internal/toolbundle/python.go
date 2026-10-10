package toolbundle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

type pythonEntry struct {
	kind   byte
	target string
	mode   fs.FileMode
	staged string
}

// extractPython retains the complete install_only tree. It validates the entire
// archive namespace before writing output, then materializes bounded file aliases
// as regular files so manifests and immutable volumes need no symlink exceptions.
func extractPython(root *os.Root, source io.Reader) error {
	stage, err := os.MkdirTemp("", "anchorage-python-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged, err := os.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer staged.Close()
	gz, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	entries := map[string]pythonEntry{}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := ValidPath(header.Name); err != nil {
			return err
		}
		full := path.Clean(header.Name)
		if full == "python" && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("Python archive root must be a directory")
		}
		if full != "python" && !strings.HasPrefix(full, "python/") {
			return fmt.Errorf("unexpected Python archive prefix %q", full)
		}
		name := strings.TrimPrefix(full, "python/")
		if full == "python" {
			name = "."
		}
		first := strings.Split(name, "/")[0]
		if name != "." && (strings.HasPrefix(first, ".") || first == "manifest.json" || first == "SHA256SUMS") {
			return fmt.Errorf("reserved Python bundle path %q", name)
		}
		if _, ok := entries[name]; ok {
			return fmt.Errorf("duplicate Python archive path %q", name)
		}
		entry := pythonEntry{kind: header.Typeflag, mode: 0644}
		if header.Mode&0111 != 0 {
			entry.mode = 0755
		}
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg, tar.TypeRegA:
			entry.staged = fmt.Sprint(len(entries))
			if err := WriteFile(staged, entry.staged, archive, 0644); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			link := header.Linkname
			if link == "" || strings.HasPrefix(link, "/") || strings.Contains(link, "\\") || strings.ContainsAny(link, "\x00\r\n") {
				return fmt.Errorf("unsafe Python link %q", link)
			}
			var parts []string
			if header.Typeflag == tar.TypeSymlink {
				parts = strings.Split(path.Dir(full), "/")
			}
			for _, part := range strings.Split(link, "/") {
				switch part {
				case "", ".":
				case "..":
					if len(parts) <= 1 {
						return fmt.Errorf("Python link escapes runtime: %q", link)
					}
					parts = parts[:len(parts)-1]
				default:
					if len(parts) == 0 && part != "python" {
						return fmt.Errorf("Python link escapes runtime: %q", link)
					}
					parts = append(parts, part)
				}
			}
			target := strings.Join(parts, "/")
			if !strings.HasPrefix(target, "python/") {
				return fmt.Errorf("Python link escapes runtime: %q", link)
			}

			entry.target = strings.TrimPrefix(target, "python/")
		default:
			return fmt.Errorf("unsupported Python archive entry %q", name)
		}
		entries[name] = entry
	}
	// Consume the gzip trailer before making the staged tree visible.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return err
	}
	for name := range entries {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if entry, ok := entries[parent]; ok && entry.kind != tar.TypeDir {
				return fmt.Errorf("non-directory Python archive parent %q", parent)
			}
		}
	}
	resolved := map[string]pythonEntry{}
	// Iterative resolution bounds work and stack use even for hostile link chains.
	resolve := func(name string) (pythonEntry, error) {
		chain := []string{}
		visiting := map[string]bool{}
		for {
			if entry, ok := resolved[name]; ok {
				for _, alias := range chain {
					resolved[alias] = entry
				}
				return entry, nil
			}
			if visiting[name] {
				return pythonEntry{}, fmt.Errorf("cyclic Python link %q", name)
			}
			visiting[name] = true
			chain = append(chain, name)
			entry, ok := entries[name]
			if !ok {
				return pythonEntry{}, fmt.Errorf("missing Python link target %q", name)
			}
			switch entry.kind {
			case tar.TypeReg, tar.TypeRegA:
				for _, alias := range chain {
					resolved[alias] = entry
				}
				return entry, nil
			case tar.TypeSymlink, tar.TypeLink:
				name = entry.target
			default:
				return pythonEntry{}, fmt.Errorf("Python link target is not a regular file %q", name)
			}
		}
	}
	var names []string
	for name, entry := range entries {
		names = append(names, name)
		if entry.kind != tar.TypeDir {
			if _, err := resolve(name); err != nil {
				return err
			}
		}
	}
	interpreter, ok := resolved["bin/python3"]
	if !ok || interpreter.mode&0111 == 0 {
		return fmt.Errorf("Python archive requires executable bin/python3")
	}
	sort.Strings(names)
	for _, name := range names {
		if entries[name].kind == tar.TypeDir {
			if err := MakeDirectory(root, name); err != nil {
				return err
			}
			continue
		}
		entry := resolved[name]
		f, err := OpenFile(staged, entry.staged)
		if err != nil {
			return err
		}
		err = WriteFile(root, name, f, entry.mode)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
