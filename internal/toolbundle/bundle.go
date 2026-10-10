package toolbundle

import (
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
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type Spec struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
	SHA512  string `json:"sha512,omitempty"`
}
type Lock struct {
	Schema    int                        `json:"schema"`
	Platforms map[string]map[string]Spec `json:"platforms"`
}

func ReadLock(r io.Reader, arch string) (map[string]Spec, error) {
	if arch != "amd64" && arch != "arm64" {
		return nil, fmt.Errorf("unsupported architecture %q", arch)
	}
	var lock Lock
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(&lock); err != nil {
		return nil, fmt.Errorf("decode lock: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing lock data")
	}
	if lock.Schema != 1 {
		return nil, fmt.Errorf("unsupported lock schema %d", lock.Schema)
	}
	tools, ok := lock.Platforms[arch]
	if !ok {
		return nil, fmt.Errorf("missing architecture %q", arch)
	}
	if len(tools) != 11 {
		return nil, fmt.Errorf("lock must contain codex, claude, gh, agy, jq, rg, fd, sqlite3, ast-grep, yq and uv")
	}
	for _, name := range []string{"codex", "claude", "gh", "agy", "jq", "rg", "fd", "sqlite3", "ast-grep", "yq", "uv"} {
		spec, ok := tools[name]
		if !ok || spec.Version == "" {
			return nil, fmt.Errorf("missing tool/version %s", name)
		}
		u, err := url.Parse(spec.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return nil, fmt.Errorf("invalid URL for %s", name)
		}
		if _, _, err := checksum(spec); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return tools, nil
}
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
func Build(ctx context.Context, client *http.Client, tools map[string]Spec, arch, destination string, output io.Writer) error {
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
	for _, name := range []string{"agy", "ast-grep", "claude", "codex", "fd", "gh", "jq", "rg", "uv", "yq"} {
		spec := tools[name]
		fmt.Fprintln(output, "Installing", name, spec.Version)
		if err := install(ctx, client, root, name, spec); err != nil {
			return fmt.Errorf("install %s: %w", name, err)
		}
	}
	return WriteManifest(root, arch, tools)
}
func install(ctx context.Context, client *http.Client, root *os.Root, name string, spec Spec) error {
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
	if name == "sqlite3" || name == "ast-grep" {
		info, err := payload.Stat()
		if err != nil {
			return err
		}
		if name == "ast-grep" {
			return extractSelectedZIP(root, payload, info.Size(), map[string]zipTarget{"ast-grep": {"bin/ast-grep", 0755}})
		}
		return extractSQLiteSource(root, payload, info.Size())
	}
	if name == "claude" || name == "jq" || name == "yq" {
		return WriteFile(root, "bin/"+name, payload, 0755)
	}
	return Extract(root, name, payload)
}

// Extract preserves all Codex resources; other tools select one executable by basename.
// fd is also installed as fdfind, using a regular file for shared-volume integrity.
// Every archive entry is validated, including entries not selected for installation.
func Extract(root *os.Root, tool string, source io.Reader) error {
	if tool != "codex" && tool != "gh" && tool != "agy" && tool != "rg" && tool != "fd" && tool != "uv" {
		return fmt.Errorf("unsupported archive tool %q", tool)
	}
	gz, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	seen := make(map[string]bool)
	count := 0
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
			if tool == "codex" {
				if err := mkdir(root, name); err != nil {
					return err
				}
			}
			continue
		}
		if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeRegA {
			return fmt.Errorf("unsupported archive entry %q", name)
		}
		target := name
		if tool != "codex" {
			executable := tool
			if tool == "agy" {
				executable = "antigravity"
			}
			if path.Base(name) == executable || tool == "uv" && path.Base(name) == "uvx" {
				target = "bin/" + tool
				if tool == "uv" {
					target = "bin/" + path.Base(name)
				}
				count++
				if entry.Mode&0111 == 0 {
					return fmt.Errorf("%s entrypoint is not executable", tool)
				}
			} else if (tool == "rg" || tool == "fd") && isLicense(path.Base(name)) {
				target = "licenses/" + tool + "/" + path.Base(name)
			} else {
				continue
			}
		} else if name == "bin/codex" {
			count++
			if entry.Mode&0111 == 0 {
				return fmt.Errorf("Codex entrypoint is not executable")
			}
		}
		mode := fs.FileMode(0644)
		if strings.HasPrefix(target, "bin/") || tool == "codex" && entry.Mode&0111 != 0 {
			mode = 0755
		}
		if err := WriteFile(root, target, archive, mode); err != nil {
			return err
		}
	}
	// Drain through gzip's trailer so truncated streams and CRC errors are reported.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return err
	}
	expected := 1
	if tool == "uv" {
		expected = 2
	}
	if count != expected {
		return fmt.Errorf("expected %d %s executable(s), found %d", expected, tool, count)
	}
	if tool == "fd" {
		f, err := OpenFile(root, "bin/fd")
		if err != nil {
			return err
		}
		defer f.Close()
		return WriteFile(root, "bin/fdfind", f, 0755)
	}
	return nil
}

func isLicense(name string) bool {
	return name == "LICENSE-MIT" || name == "LICENSE-APACHE" || name == "UNLICENSE" || name == "COPYING"
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
