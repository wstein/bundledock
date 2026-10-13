package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type gitTrimReceipt struct {
	Spec  Spec              `json:"source_pin"`
	Arch  string            `json:"architecture"`
	Files map[string]string `json:"compiled_files_sha256"`
}

// PrepareGitTrim only stages verified source. The fixed Docker recipe owns
// network fetch/vendoring and compilation; this function never executes it.
func PrepareGitTrim(ctx context.Context, spec Spec, destination string) error {
	if err := toolconfig.ValidateGitTrimSpec(spec); err != nil {
		return err
	}
	client := &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return fmt.Errorf("git-trim source redirects are forbidden")
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("git-trim source HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return err
	}
	files, err := toolconfig.ReadGitTrimArchive(data, spec.SHA256)
	if err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	for n, b := range files {
		if err := WriteFile(root, n, bytes.NewReader(b), 0644); err != nil {
			return err
		}
	}
	return nil
}

func gitTrimBinary(data []byte, arch string) error {
	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("git-trim ELF: %w", err)
	}
	defer f.Close()
	machine := elf.EM_X86_64
	if arch == "arm64" {
		machine = elf.EM_AARCH64
	} else if arch != "amd64" {
		return fmt.Errorf("unsupported git-trim architecture")
	}
	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Machine != machine || (f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN) {
		return fmt.Errorf("git-trim ELF architecture/type mismatch")
	}
	libs, err := f.ImportedLibraries()
	if err != nil {
		return err
	}
	for _, lib := range libs {
		switch lib {
		case "libc.so.6", "libgcc_s.so.1", "libm.so.6", "libpthread.so.0", "libdl.so.2", "ld-linux-x86-64.so.2", "ld-linux-aarch64.so.1":
		default:
			return fmt.Errorf("git-trim unexpected runtime dependency %s", lib)
		}
	}
	return nil
}

func regularBytes(root *os.Root, n string, limit int64) ([]byte, error) {
	info, err := root.Lstat(n)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("invalid staged git-trim file %s", n)
	}
	f, err := root.Open(n)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("staged git-trim file too large")
	}
	return b, err
}

// SealGitTrim records measured output hashes separately from upstream source
// checksums. It retains all discovered vendored and native-library license files.
func SealGitTrim(spec Spec, arch, source, stage string) error {
	return sealGitTrim(spec, arch, source, stage, os.DirFS("/"), os.DirFS("/build/licenses/git-trim/dependencies"))
}

func sealGitTrim(spec Spec, arch, source, stage string, native, fallback fs.FS) error {
	if err := toolconfig.ValidateGitTrimSpec(spec); err != nil {
		return err
	}
	r, err := os.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer r.Close()
	b, err := regularBytes(r, "bin/git-trim", 64<<20)
	if err != nil {
		return err
	}
	if err := gitTrimBinary(b, arch); err != nil {
		return err
	}
	lock, err := os.ReadFile(filepath.Join(source, "Cargo.lock"))
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(lock)) != spec.Source.CargoLockSHA256 {
		return fmt.Errorf("git-trim Cargo.lock changed")
	}
	// The exact reviewed Cargo.lock uses these canonical registry package blocks.
	blocks := regexp.MustCompile(`(?m)^name = "([A-Za-z0-9_-]+)"\nversion = "([A-Za-z0-9.+_-]+)"\nsource = "registry\+https://github.com/rust-lang/crates.io-index"\nchecksum = "([a-f0-9]{64})"`).FindAllSubmatch(lock, -1)
	if len(blocks) != 117 {
		return fmt.Errorf("git-trim dependency closure changed")
	}
	for _, block := range blocks {
		name, version, sum := string(block[1]), string(block[2]), string(block[3])
		vendor := filepath.Join(source, "vendor", name+"-"+version)
		data, err := os.ReadFile(filepath.Join(vendor, ".cargo-checksum.json"))
		if err != nil {
			return err
		}
		var checks struct {
			Package string            `json:"package"`
			Files   map[string]string `json:"files"`
		}
		if err := toolconfig.Decode(bytes.NewReader(data), &checks); err != nil {
			return err
		}
		if checks.Package != sum {
			return fmt.Errorf("git-trim vendored checksum mismatch for %s", name)
		}
		notices := 0
		err = filepath.WalkDir(vendor, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("nonregular vendored source")
			}
			rel, err := filepath.Rel(vendor, path)
			if err != nil {
				return err
			}
			upper := strings.ToUpper(d.Name())
			isNotice := strings.Contains(upper, "LICENSE") || strings.Contains(upper, "LICENCE") || strings.HasPrefix(upper, "COPYING") || strings.HasPrefix(upper, "NOTICE") || strings.HasPrefix(upper, "COPYRIGHT")
			if !isNotice {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if len(b) > 8<<20 {
				return fmt.Errorf("oversized dependency notice")
			}
			if fmt.Sprintf("%x", sha256.Sum256(b)) != checks.Files[filepath.ToSlash(rel)] {
				return fmt.Errorf("changed vendored notice %s", rel)
			}
			notices++
			return WriteFile(r, "licenses/git-trim/dependencies/"+name+"-"+version+"/"+filepath.ToSlash(rel), bytes.NewReader(b), 0644)
		})
		if err != nil {
			return err
		}
		if notices == 0 {
			// A few official crates omit their full license files. Version-bound
			// upstream notices from the crate's recorded source commit are retained
			// in BundleDock source and copied by the trusted fixed Docker stage.
			dir := name + "-" + version
			entries, err := fs.ReadDir(fallback, dir)
			if err != nil {
				return fmt.Errorf("git-trim dependency %s has no full license notice: %w", name, err)
			}
			for _, entry := range entries {
				if !entry.Type().IsRegular() {
					return fmt.Errorf("invalid fallback license")
				}
				b, err := fs.ReadFile(fallback, dir+"/"+entry.Name())
				if err != nil {
					return err
				}
				if err := WriteFile(r, "licenses/git-trim/dependencies/"+dir+"/"+entry.Name(), bytes.NewReader(b), 0644); err != nil {
					return err
				}
				if strings.HasPrefix(entry.Name(), "LICENSE") {
					notices++
				}
			}
			if notices == 0 {
				return fmt.Errorf("missing full fallback license")
			}
		}
	}
	// Statically linked native and Rust runtime notices come from the immutable
	// compiler image, independently of the Cargo registry dependency closure.
	for name, sourcePath := range gitTrimRuntimeNotices(arch) {
		b, err := fs.ReadFile(native, sourcePath)
		if err != nil {
			return err
		}
		if err := WriteFile(r, "licenses/git-trim/native/"+name, bytes.NewReader(b), 0644); err != nil {
			return err
		}
	}
	status, err := fs.ReadFile(native, "var/lib/dpkg/status")
	if err != nil {
		return err
	}
	packageInfo := ""
	for _, block := range strings.Split(string(status), "\n\n") {
		if strings.HasPrefix(block, "Package: libssl-dev\n") &&
			strings.Contains(block, "\nStatus: install ok installed\n") &&
			strings.Contains(block, "\nArchitecture: "+arch+"\n") &&
			strings.Contains(block, "\nVersion: ") {
			packageInfo = block
			break
		}
	}
	if packageInfo == "" {
		return fmt.Errorf("missing installed libssl-dev version provenance")
	}
	provenance := "Builder: " + toolconfig.GitTrimBuilder + "\n\n" + packageInfo + "\n"
	if err := WriteFile(r, "licenses/git-trim/native/libssl-dev-SOURCE", strings.NewReader(provenance), 0644); err != nil {
		return err
	}
	receipt := gitTrimReceipt{Spec: spec, Arch: arch, Files: map[string]string{}}
	err = fs.WalkDir(r.FS(), ".", func(n string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if n == "provenance.json" {
			return fmt.Errorf("git-trim stage already sealed")
		}
		b, err := regularBytes(r, n, 64<<20)
		if err != nil {
			return err
		}
		receipt.Files[n] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(r, "provenance.json", bytes.NewReader(append(data, '\n')), 0644)
}

func gitTrimRuntimeNotices(arch string) map[string]string {
	triple := "x86_64-unknown-linux-gnu"
	if arch == "arm64" {
		triple = "aarch64-unknown-linux-gnu"
	}
	paths := map[string]string{
		"libssl-dev-copyright":   "usr/share/doc/libssl-dev/copyright",
		"OpenSSL-LICENSE-APACHE": "usr/share/common-licenses/Apache-2.0",
		"zlib1g-dev-copyright":   "usr/share/doc/zlib1g-dev/copyright",
	}
	for _, name := range []string{"COPYRIGHT", "LICENSE-MIT", "LICENSE-APACHE"} {
		paths["rust-"+name] = "usr/local/rustup/toolchains/1.75.0-" + triple + "/share/doc/rust/" + name
	}
	return paths
}

func installGitTrimStage(destination *os.Root, stage, arch string, spec Spec) error {
	if err := toolconfig.ValidateGitTrimSpec(spec); err != nil {
		return err
	}
	r, err := os.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := regularBytes(r, "provenance.json", 1<<20)
	if err != nil {
		return err
	}
	var receipt gitTrimReceipt
	if err := toolconfig.Decode(bytes.NewReader(data), &receipt); err != nil {
		return err
	}
	if err := toolconfig.ValidateGitTrimSpec(receipt.Spec); err != nil {
		return err
	}
	if receipt.Arch != arch || len(receipt.Files) < 3 {
		return fmt.Errorf("staged git-trim provenance mismatch")
	}
	if _, ok := receipt.Files["bin/git-trim"]; !ok {
		return fmt.Errorf("missing staged git-trim binary")
	}
	count := 0
	err = fs.WalkDir(r.FS(), ".", func(n string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if n == "provenance.json" {
			return nil
		}
		if n != "bin/git-trim" && !strings.HasPrefix(n, "licenses/git-trim/") {
			return fmt.Errorf("unexpected staged git-trim member")
		}
		b, err := regularBytes(r, n, 64<<20)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != receipt.Files[n] {
			return fmt.Errorf("staged git-trim checksum mismatch")
		}
		mode := fs.FileMode(0644)
		if n == "bin/git-trim" {
			if err := gitTrimBinary(b, arch); err != nil {
				return err
			}
			mode = 0755
		}
		count++
		return WriteFile(destination, n, bytes.NewReader(b), mode)
	})
	if err != nil {
		return err
	}
	if count != len(receipt.Files) {
		return fmt.Errorf("incomplete staged git-trim payload")
	}
	return WriteFile(destination, "licenses/git-trim/BUILD-PROVENANCE.json", bytes.NewReader(data), 0644)
}
