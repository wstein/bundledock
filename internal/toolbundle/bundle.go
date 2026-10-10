package toolbundle

import (
	"anchorage/internal/toolconfig"
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type Spec = toolconfig.Spec

func checksum(spec Spec) (hash.Hash, string, error) {
	var h hash.Hash
	var expected string
	switch {
	case spec.SHA256 != "" && spec.SHA512 == "":
		h, expected = sha256.New(), spec.SHA256
	case spec.SHA512 != "" && spec.SHA256 == "":
		h, expected = sha512.New(), spec.SHA512
	default:
		return nil, "", fmt.Errorf("exactly one checksum required")
	}
	b, err := hex.DecodeString(expected)
	if err != nil || len(b) != h.Size() {
		return nil, "", fmt.Errorf("invalid checksum")
	}
	return h, strings.ToLower(expected), nil
}

// Build downloads each artifact to a temporary file, verifies it, then extracts.
// SQLite must already be compiled for arch at destination/bin/sqlite3.
// A failed build leaves its incomplete output for inspection; use a fresh directory.
func Build(ctx context.Context, client *http.Client, tools map[string]Spec, packages map[string]toolconfig.Package, notices fs.FS, arch, destination string, output io.Writer) error {
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
	sqlite, err := OpenFile(root, "bin/sqlite3")
	if err != nil {
		return fmt.Errorf("compiled SQLite: %w", err)
	}
	info, err := sqlite.Stat()
	closeErr := sqlite.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if info.Mode().Perm()&0111 == 0 || info.Size() == 0 {
		return fmt.Errorf("compiled SQLite must be a nonempty executable")
	}
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if packages[name].Install.Kind == "sqlite-source" {
			continue
		}
		spec := tools[name]
		fmt.Fprintln(output, "Installing", name, spec.Version)
		if err := install(ctx, client, root, name, packages[name], spec); err != nil {
			return fmt.Errorf("install %s: %w", name, err)
		}
	}
	for name, p := range packages {
		for _, notice := range p.Notices {
			f, err := notices.Open(notice)
			if err != nil {
				return err
			}
			err = WriteFile(root, "licenses/"+name+"/"+path.Base(notice), f, 0644)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return WriteManifest(root, arch, tools)
}
func install(ctx context.Context, client *http.Client, root *os.Root, name string, p toolconfig.Package, spec Spec) error {
	h, expected, err := checksum(spec)
	if err != nil {
		return err
	}
	payload, err := os.CreateTemp("", "anchorage-tool-*")
	if err != nil {
		return err
	}
	defer os.Remove(payload.Name())
	defer payload.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return fmt.Errorf("download HTTP %s", response.Status)
	}
	_, copyErr := io.Copy(io.MultiWriter(payload, h), response.Body)
	closeErr := response.Body.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return fmt.Errorf("checksum mismatch")
	}
	if _, err := payload.Seek(0, io.SeekStart); err != nil {
		return err
	}
	switch p.Install.Kind {
	case "zip", "sqlite-source":
		info, err := payload.Stat()
		if err != nil {
			return err
		}
		selected := map[string]zipTarget{}
		for member, target := range p.Install.Members {
			mode := fs.FileMode(0755)
			if p.Install.Kind == "sqlite-source" {
				mode = 0644
			}
			selected[member] = zipTarget{target, mode}
		}
		if err := extractSelectedZIP(root, payload, info.Size(), selected); err != nil {
			return err
		}
		return copyAliases(root, p.Install.Aliases)
	case "raw":
		if len(p.Install.Members) != 1 {
			return fmt.Errorf("raw recipe requires one executable")
		}
		for _, target := range p.Install.Members {
			if err := WriteFile(root, target, payload, 0755); err != nil {
				return err
			}
		}
		return copyAliases(root, p.Install.Aliases)
	case "tar.gz":
		return Extract(root, name, p, payload)
	default:
		return fmt.Errorf("unsupported install kind %q", p.Install.Kind)
	}
}

// Extract validates every tar member before selecting declarative executables.
// Preserve keeps native package resources; aliases are independent regular files.
func Extract(root *os.Root, tool string, p toolconfig.Package, source io.Reader) error {
	if p.Install.Kind != "tar.gz" {
		return fmt.Errorf("expected tar.gz recipe")
	}
	gz, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	seen := map[string]bool{}
	installed := map[string]bool{}
	for {
		entry, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := ValidPath(entry.Name); err != nil {
			return err
		}
		name := path.Clean(entry.Name)
		if seen[name] {
			return fmt.Errorf("duplicate archive path %q", name)
		}
		seen[name] = true
		if entry.Typeflag == tar.TypeDir {
			if p.Install.Preserve {
				if err := mkdir(root, name); err != nil {
					return err
				}
			}
			continue
		}
		if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeRegA {
			return fmt.Errorf("unsupported archive entry %q", name)
		}
		key := path.Base(name)
		if p.Install.Preserve {
			key = name
		}
		target, selected := p.Install.Members[key]
		if selected {
			if installed[key] {
				return fmt.Errorf("duplicate archive selection %q", key)
			}
			installed[key] = true
			if entry.Mode&0111 == 0 {
				return fmt.Errorf("%s entrypoint is not executable", tool)
			}
		} else if p.Install.Preserve {
			target = name
		} else {
			for _, notice := range p.Install.ArchiveNotices {
				if path.Base(name) == notice {
					target = "licenses/" + tool + "/" + notice
					break
				}
			}
			if target == "" {
				continue
			}
		}
		mode := fs.FileMode(0644)
		if selected || p.Install.Preserve && entry.Mode&0111 != 0 {
			mode = 0755
		}
		if err := WriteFile(root, target, archive, mode); err != nil {
			return err
		}
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return err
	}
	if len(installed) != len(p.Install.Members) {
		return fmt.Errorf("expected %d %s executable(s), found %d", len(p.Install.Members), tool, len(installed))
	}
	return copyAliases(root, p.Install.Aliases)
}
func copyAliases(root *os.Root, aliases map[string]string) error {
	for alias, target := range aliases {
		f, err := OpenFile(root, target)
		if err != nil {
			return err
		}
		err = WriteFile(root, alias, f, 0755)
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

func WriteManifest(root *os.Root, arch string, tools map[string]Spec) error {
	manifest := struct {
		Schema   int             `json:"schema"`
		Platform string          `json:"platform"`
		Tools    map[string]Spec `json:"tools"`
	}{1, "linux/" + arch, tools}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := WriteFile(root, "manifest.json", bytes.NewReader(data), 0644); err != nil {
		return err
	}
	var names []string
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported bundle file %q", name)
		}
		if name != "SHA256SUMS" {
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(names)
	var sums bytes.Buffer
	for _, name := range names {
		f, err := OpenFile(root, name)
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
		fmt.Fprintf(&sums, "%x  %s\n", h.Sum(nil), name)
	}
	return WriteFile(root, "SHA256SUMS", &sums, 0644)
}
