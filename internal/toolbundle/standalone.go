package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
)

const StandaloneProfile = "code-server-standalone"

var standaloneRoot = regexp.MustCompile(`^code-server-[0-9]+\.[0-9]+\.[0-9]+-linux-(amd64|arm64)$`)

// LinkTarget validates a canonical relative link to a direct regular payload file.
// It never follows a symbolic link, including in the target's parent components.
func LinkTarget(root *os.Root, name, target string) error {
	if name == "payload/bunx" {
		return bunLinkTarget(root, name, target)
	}
	if strings.HasPrefix(name, "payload/bin/") {
		return nodeLinkTarget(root, name, target)
	}
	return standaloneLinkTarget(root, name, target)
}
func standaloneLinkTarget(root *os.Root, name, target string) error {
	if err := ValidPath(name); err != nil {
		return err
	}
	if path.Clean(name) != name || !strings.HasPrefix(name, "payload/node_modules/.bin/") || path.Dir(name) != "payload/node_modules/.bin" {
		return fmt.Errorf("unsupported standalone link %q", name)
	}
	if target == "" || path.IsAbs(target) || path.Clean(target) != target || strings.ContainsAny(target, "\\\r\n\x00") {
		return fmt.Errorf("unsafe link target %q", target)
	}
	resolved := path.Join(path.Dir(name), target)
	if !strings.HasPrefix(resolved, "payload/node_modules/") {
		return fmt.Errorf("link escapes payload: %q", name)
	}
	f, err := OpenFile(root, resolved)
	if err != nil {
		return err
	}
	return f.Close()
}

// CheckLink validates the leaf without following it; all parents remain link-free.
func CheckLink(root *os.Root, name, target string) error {
	if err := LinkTarget(root, name, target); err != nil {
		return err
	}
	if err := CheckPath(root, path.Dir(name)); err != nil {
		return err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("expected symbolic link %q", name)
	}
	actual, err := root.Readlink(name)
	if err != nil {
		return err
	}
	if actual != target {
		return fmt.Errorf("link target differs: %q", name)
	}
	return nil
}

// OpenContent hashes literal link text, never the file reached by a link.
func OpenContent(root *os.Root, name string, links map[string]string) (io.ReadCloser, error) {
	if target, ok := links[name]; ok {
		if err := CheckLink(root, name, target); err != nil {
			return nil, err
		}
		return io.NopCloser(strings.NewReader(target)), nil
	}
	return OpenFile(root, name)
}

func extractStandalone(root *os.Root, source io.Reader, expectedRoot string) error {
	return extractPayload(root, source, expectedRoot, StandaloneProfile)
}
func extractPayload(root *os.Root, source io.Reader, expectedRoot, profile string) error {
	gz, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gz.Close()
	limited := &io.LimitedReader{R: gz, N: 2<<30 + 1}
	tr := tar.NewReader(limited)
	seen := map[string]bool{}
	links := map[string]string{}
	prefix := ""
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := ValidPath(h.Name); err != nil {
			return err
		}
		clean := path.Clean(h.Name)
		parts := strings.SplitN(clean, "/", 2)
		validRoot := standaloneRoot.MatchString(parts[0])
		if profile == NodeProfile {
			validRoot = nodeRoot.MatchString(parts[0])
		}
		if !validRoot || prefix != "" && prefix != parts[0] || expectedRoot != "" && parts[0] != expectedRoot {
			return fmt.Errorf("invalid standalone archive root %q", h.Name)
		}
		prefix = parts[0]
		name := "payload"
		if len(parts) == 2 {
			name += "/" + parts[1]
		}
		if seen[name] || len(seen) >= 32768 || h.Size > 512<<20 {
			return fmt.Errorf("duplicate or oversized standalone member %q", name)
		}
		seen[name] = true
		if profile == NodeProfile && (name == "payload/bin/corepack" || name == "payload/lib/node_modules/corepack" || strings.HasPrefix(name, "payload/lib/node_modules/corepack/")) {
			if name == "payload/bin/corepack" {
				if h.Typeflag != tar.TypeSymlink || h.Linkname != "../lib/node_modules/corepack/dist/corepack.js" {
					return fmt.Errorf("invalid omitted corepack link")
				}
			} else if h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
				return fmt.Errorf("unsupported omitted corepack member")
			}
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := mkdir(root, name); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			mode := fs.FileMode(0644)
			if h.Mode&0111 != 0 {
				mode = 0755
			}
			if err := WriteFile(root, name, tr, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			links[name] = h.Linkname
		default:
			return fmt.Errorf("unsupported standalone member %q", name)
		}
	}
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return err
	}
	if limited.N == 0 {
		return fmt.Errorf("standalone archive too large")
	}
	for name, target := range links {
		for candidate := range seen {
			if strings.HasPrefix(candidate, name+"/") {
				return fmt.Errorf("archive writes through link %q", name)
			}
		}
		if err := profileLinkTarget(root, profile, name, target); err != nil {
			return err
		}
		if err := CheckPath(root, name); err != nil {
			return err
		}
		if _, err := root.Lstat(name); !os.IsNotExist(err) {
			return fmt.Errorf("link collision %q", name)
		}
	}
	executables := []string{"payload/bin/code-server", "payload/lib/node"}
	if profile == NodeProfile {
		executables = []string{"payload/bin/node", "payload/lib/node_modules/npm/bin/npm-cli.js", "payload/lib/node_modules/npm/bin/npx-cli.js"}
		if len(links) != 2 {
			return fmt.Errorf("Node requires npm and npx links")
		}
	}
	for _, name := range executables {
		f, err := OpenFile(root, name)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		f.Close()
		if err != nil {
			return err
		}
		if info.Mode()&0111 == 0 {
			return fmt.Errorf("standalone executable missing mode: %s", name)
		}
	}
	for name, target := range links {
		if err := mkdir(root, path.Dir(name)); err != nil {
			return err
		}
		if err := root.Symlink(target, name); err != nil {
			return err
		}
	}
	if profile == NodeProfile {
		return writeNodeLaunchers(root)
	}
	return WriteFile(root, "bin/code-server", strings.NewReader("#!/bin/sh\nset -eu\nexec \"${0%/*}/../payload/bin/code-server\" \"$@\"\n"), 0755)
}

// BundleLinks requires explicit caller opt-in and matching, validated metadata.
// The caller's immutable image identity is the trust anchor, not these fields.
func BundleLinks(root *os.Root, profile string) (map[string]string, error) {
	f, err := OpenFile(root, "manifest.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var m struct {
		Schema   int               `json:"schema"`
		Profile  string            `json:"profile"`
		Links    map[string]string `json:"links"`
		Platform string            `json:"platform"`
		Tools    map[string]Spec   `json:"tools"`
	}
	if err := toolconfig.Decode(f, &m); err != nil {
		return nil, err
	}
	if profile == "" {
		if m.Profile != "" || len(m.Links) != 0 {
			return nil, fmt.Errorf("bundle profile requires explicit opt-in")
		}
		return nil, nil
	}
	if (profile != StandaloneProfile && profile != NodeProfile && profile != BunProfile) || m.Profile != profile || m.Schema != 1 || len(m.Tools) != 1 {
		return nil, fmt.Errorf("invalid standalone manifest profile")
	}
	arch := strings.TrimPrefix(m.Platform, "linux/")
	tool := "code-server"
	validate := ValidateStandaloneSpec
	if profile == BunProfile {
		tool = "bun"
		validate = ValidateBunSpec
	}
	if profile == NodeProfile {
		tool = "node"
		validate = ValidateNodeSpec
	}
	s, ok := m.Tools[tool]
	if !ok || arch != "amd64" && arch != "arm64" || validate(s, arch) != nil {
		return nil, fmt.Errorf("invalid standalone provenance")
	}
	if profile == NodeProfile && (len(m.Links) != 2 || m.Platform != "linux/"+arch) {
		return nil, fmt.Errorf("Node requires two links")
	}
	if profile == BunProfile && (len(m.Links) != 1 || m.Links["payload/bunx"] != "bun" || m.Platform != "linux/"+arch) {
		return nil, fmt.Errorf("Bun requires its exact bunx link")
	}
	for name, target := range m.Links {
		if err := profileLinkTarget(root, profile, name, target); err != nil {
			return nil, err
		}
		if err := CheckLink(root, name, target); err != nil {
			return nil, err
		}
	}
	return m.Links, nil
}

func ValidateStandaloneSpec(s Spec, arch string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("invalid standalone architecture")
	}
	if err := toolconfig.ValidateSpec(s); err != nil {
		return err
	}
	version := strings.TrimPrefix(s.Version, "v")
	if _, err := toolconfig.ParseVersion(version); err != nil {
		return err
	}
	expected := "https://github.com/coder/code-server/releases/download/v" + version + "/code-server-" + version + "-linux-" + arch + ".tar.gz"
	if s.Version != "v"+version || s.URL != expected {
		return fmt.Errorf("invalid code-server release URL")
	}
	return nil
}

func WriteStandaloneManifest(root *os.Root, arch string, tools map[string]Spec) error {
	return writePayloadManifest(root, arch, tools, StandaloneProfile)
}
func writePayloadManifest(root *os.Root, arch string, tools map[string]Spec, profile string) error {
	if len(tools) != 1 {
		return fmt.Errorf("standalone manifest requires code-server only")
	}
	tool := "code-server"
	validate := ValidateStandaloneSpec
	if profile == BunProfile {
		tool = "bun"
		validate = ValidateBunSpec
	}
	if profile == NodeProfile {
		tool = "node"
		validate = ValidateNodeSpec
	}
	if err := validate(tools[tool], arch); err != nil {
		return err
	}
	links := map[string]string{}
	err := fs.WalkDir(root.FS(), ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.Type()&os.ModeSymlink != 0 {
			target, err := root.Readlink(name)
			if err != nil {
				return err
			}
			if err := profileLinkTarget(root, profile, name, target); err != nil {
				return err
			}
			if err := CheckLink(root, name, target); err != nil {
				return err
			}
			links[name] = target
		}
		return nil
	})
	if err != nil {
		return err
	}
	if profile == NodeProfile && len(links) != 2 {
		return fmt.Errorf("Node requires npm and npx links")
	}
	if profile == BunProfile && (len(links) != 1 || links["payload/bunx"] != "bun") {
		return fmt.Errorf("Bun requires its exact bunx link")
	}
	return writeManifest(root, arch, tools, profile, links)
}
