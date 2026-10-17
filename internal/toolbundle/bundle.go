package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
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

	"github.com/ulikunitz/xz"
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
// A failed build leaves its incomplete output for inspection; use a fresh directory.
// It never invokes a compiler or executes catalog-provided commands.
func Build(ctx context.Context, client *http.Client, tools map[string]Spec, packages map[string]toolconfig.Package, notices fs.FS, arch, destination string, output io.Writer, opts ...BuildOption) error {
	var o buildOptions
	for _, opt := range opts {
		opt(&o)
	}
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
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := directory.ReadDir(1)
	closeErr := directory.Close()
	if readErr != nil && readErr != io.EOF {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) != 0 {
		return fmt.Errorf("bundle destination must be empty; preserve existing files and use a fresh directory")
	}
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec := tools[name]
		fmt.Fprintln(output, "Installing", name, spec.Version)
		if err := install(ctx, client, root, name, packages[name], spec); err != nil {
			return fmt.Errorf("install %s: %w", name, err)
		}
	}
	if err := shellLayerStep(root, packages, o); err != nil {
		return fmt.Errorf("shell layer: %w", err)
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
	if err := WriteLicenseArchive(root); err != nil {
		return err
	}
	if len(packages) == 1 && packages["code-server"].Install.Profile == StandaloneProfile {
		return WriteStandaloneManifest(root, arch, tools)
	}
	if len(packages) == 1 && packages["node"].Install.Profile == NodeProfile {
		return writePayloadManifest(root, arch, tools, NodeProfile)
	}
	if len(packages) == 1 && packages["bun"].Install.Profile == BunProfile {
		return WriteBunManifest(root, arch, tools)
	}
	return WriteManifest(root, arch, tools)
}
func install(ctx context.Context, client *http.Client, root *os.Root, name string, p toolconfig.Package, spec Spec) error {
	h, expected, err := checksum(spec)
	if err != nil {
		return err
	}
	payload, err := os.CreateTemp("", "bundledock-tool-*")
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
	var download io.Reader = response.Body
	if p.Install.Kind == "tar.xz" || p.Install.Profile == BunProfile {
		download = io.LimitReader(download, 64<<20+1)
	}
	copied, copyErr := io.Copy(io.MultiWriter(payload, h), download)
	closeErr := response.Body.Close()
	if copyErr != nil {
		return copyErr
	}
	if (p.Install.Kind == "tar.xz" || p.Install.Profile == BunProfile) && copied > 64<<20 {
		return fmt.Errorf("bounded archive download too large")
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
	case "zip":
		info, err := payload.Stat()
		if err != nil {
			return err
		}
		if p.Install.Profile == BunProfile {
			arch := "amd64"
			if strings.HasSuffix(spec.URL, "-aarch64.zip") {
				arch = "arm64"
			}
			if err := ValidateBunSpec(spec, arch); err != nil {
				return err
			}
			return extractBunZIP(root, payload, info.Size(), strings.TrimSuffix(path.Base(spec.URL), ".zip"))
		}
		selected := map[string]zipTarget{}
		for member, target := range p.Install.Members {
			mode := fs.FileMode(0755)
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
	case "tar.gz", "tar.xz":
		if p.Install.Profile == NodeProfile {
			arch := "amd64"
			if strings.HasSuffix(spec.URL, "-arm64.tar.gz") {
				arch = "arm64"
			}
			if err := ValidateNodeSpec(spec, arch); err != nil {
				return err
			}
			return extractPayload(root, payload, strings.TrimSuffix(path.Base(spec.URL), ".tar.gz"), NodeProfile)
		}
		if p.Install.Profile == StandaloneProfile {
			arch := "amd64"
			if strings.HasSuffix(spec.URL, "-arm64.tar.gz") {
				arch = "arm64"
			}
			if err := ValidateStandaloneSpec(spec, arch); err != nil {
				return err
			}
			expectedRoot := "code-server-" + strings.TrimPrefix(spec.Version, "v") + "-linux-" + arch
			return extractStandalone(root, payload, expectedRoot)
		}
		if p.Install.Profile == "helix-runtime" {
			expectedRoot := strings.TrimSuffix(path.Base(spec.URL), ".tar.xz")
			if !helixArchiveRoot.MatchString(expectedRoot) || !strings.HasPrefix(expectedRoot, "helix-"+spec.Version+"-") {
				return fmt.Errorf("invalid Helix release URL")
			}
			return extract(root, name, p, payload, expectedRoot)
		}
		return Extract(root, name, p, payload)
	default:
		return fmt.Errorf("unsupported install kind %q", p.Install.Kind)
	}
}

// Extract validates every tar member before selecting declarative executables.
// Preserve keeps native package resources except declared directories; aliases
// are independent regular files. Excluded entries undergo the same validation.
func Extract(root *os.Root, tool string, p toolconfig.Package, source io.Reader) error {
	return extract(root, tool, p, source, "")
}
func extract(root *os.Root, tool string, p toolconfig.Package, source io.Reader, expectedHelixRoot string) error {
	if p.Install.Profile == NodeProfile {
		return extractPayload(root, source, "", NodeProfile)
	}
	if p.Install.Profile == StandaloneProfile {
		return extractStandalone(root, source, "")
	}
	var decoded io.Reader
	var limited *io.LimitedReader
	switch p.Install.Kind {
	case "tar.gz":
		gz, err := gzip.NewReader(source)
		if err != nil {
			return err
		}
		defer gz.Close()
		decoded = gz
	case "tar.xz":
		// XZ headers control dictionary allocation. Production install verifies
		// the trusted upstream checksum before constructing this decoder.
		reader, err := xz.NewReader(source)
		if err != nil {
			return err
		}
		limited = &io.LimitedReader{R: reader, N: 256<<20 + 1}
		decoded = limited
	default:
		return fmt.Errorf("expected tar.gz or tar.xz recipe")
	}
	archive := tar.NewReader(decoded)
	seen := map[string]bool{}
	installed := map[string]bool{}
	treeFiles := map[string]int{}
	helixRoot := ""
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
		if limited != nil {
			if len(seen) >= 4096 {
				return fmt.Errorf("too many XZ archive members")
			}
			if entry.Size > 128<<20 {
				return fmt.Errorf("XZ archive member too large: %q", name)
			}
		}
		if seen[name] {
			return fmt.Errorf("duplicate archive path %q", name)
		}
		seen[name] = true
		if p.Install.Profile == "helix-runtime" {
			parts := strings.SplitN(name, "/", 2)
			if !helixArchiveRoot.MatchString(parts[0]) || expectedHelixRoot != "" && parts[0] != expectedHelixRoot || helixRoot != "" && helixRoot != parts[0] {
				return fmt.Errorf("invalid Helix archive root %q", name)
			}
			helixRoot = parts[0]
			if len(parts) == 1 {
				if entry.Typeflag != tar.TypeDir {
					return fmt.Errorf("Helix root must be a directory")
				}
				continue
			}
			name = parts[1]
		}
		if entry.Typeflag == tar.TypeDir {
			if p.Install.Preserve && !p.Install.Excludes(name) {
				directory := name
				if p.Install.Profile == "helix-runtime" {
					directory = "lib/helix/" + name
				}
				if err := mkdir(root, directory); err != nil {
					return err
				}
			}
			continue
		}
		if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeRegA {
			return fmt.Errorf("unsupported archive entry %q", name)
		}
		if p.Install.Excludes(name) {
			// Consume the payload through the tar reader so truncation and decoder
			// integrity failures still abort even though no output file is created.
			if _, err := io.Copy(io.Discard, archive); err != nil {
				return err
			}
			continue
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
			if p.Install.Profile == "helix-runtime" {
				target = "lib/helix/" + name
			}
		} else {
			for _, tree := range p.Install.ArchiveNoticeTrees {
				if strings.HasPrefix(name, tree+"/") {
					target = "licenses/" + tool + "/" + name
					treeFiles[tree]++
					break
				}
			}
			if target == "" {
				for _, notice := range p.Install.ArchiveNotices {
					if path.Base(name) == notice {
						target = "licenses/" + tool + "/" + notice
						break
					}
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
	if _, err := io.Copy(io.Discard, decoded); err != nil {
		return err
	}
	if limited != nil && limited.N == 0 {
		return fmt.Errorf("XZ archive decompressed data too large")
	}
	if len(installed) != len(p.Install.Members) {
		return fmt.Errorf("expected %d %s executable(s), found %d", len(p.Install.Members), tool, len(installed))
	}
	for _, tree := range p.Install.ArchiveNoticeTrees {
		if treeFiles[tree] == 0 {
			return fmt.Errorf("%s notice tree %q retained no files", tool, tree)
		}
	}
	if p.Install.Profile == "helix-runtime" {
		return writeHelixLauncher(root)
	}
	if tool == "uv" {
		if err := writeSpecifyLauncher(root); err != nil {
			return err
		}
		if err := writeTldrLauncher(root); err != nil {
			return err
		}
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
	return writeManifest(root, arch, tools, "", nil)
}
func writeManifest(root *os.Root, arch string, tools map[string]Spec, profile string, links map[string]string) error {
	launchers := map[string]string{}
	for name, pin := range map[string]string{"specify": "specify-cli==1.1.3", "tldr": "tldr==3.4.4"} {
		if _, err := root.Stat("bin/" + name); err == nil {
			launchers[name] = pin
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	manifest := struct {
		Schema    int               `json:"schema"`
		Profile   string            `json:"profile,omitempty"`
		Links     map[string]string `json:"links,omitempty"`
		Platform  string            `json:"platform"`
		Tools     map[string]Spec   `json:"tools"`
		Launchers map[string]string `json:"launchers,omitempty"`
	}{1, profile, links, "linux/" + arch, tools, launchers}
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
		if !entry.Type().IsRegular() && links[name] == "" {
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
		f, err := OpenContent(root, name, links)
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
