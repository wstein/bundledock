package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestGitTrimCannotInstallSourceAsExecutable(t *testing.T) {
	s := toolconfig.GitTrimSpec()
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	err = Build(context.Background(), nil, map[string]Spec{"git-trim": s}, map[string]toolconfig.Package{"git-trim": c.Catalog.Packages["git-trim"]}, os.DirFS("../../bundles/toolbox"), "amd64", t.TempDir(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "staged git-trim") {
		t.Fatalf("source archive must require a compiled stage before any network request: %v", err)
	}
}

func TestGitTrimSealChecksCompleteVersionedDependencyClosure(t *testing.T) {
	lock, err := os.ReadFile("../toolmanager/testdata/git-trim-Cargo.lock")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile(`(?m)^name = "([A-Za-z0-9_-]+)"\nversion = "([A-Za-z0-9.+_-]+)"\nsource = "registry\+https://github.com/rust-lang/crates.io-index"\nchecksum = "([a-f0-9]{64})"`).FindAllStringSubmatch(string(lock), -1)
	for _, scenario := range []string{"valid", "changed checksum", "missing crate", "changed license", "missing fallback"} {
		t.Run(scenario, func(t *testing.T) {
			source := t.TempDir()
			os.WriteFile(filepath.Join(source, "Cargo.lock"), lock, 0644)
			for _, block := range blocks {
				dir := filepath.Join(source, "vendor", block[1]+"-"+block[2])
				os.MkdirAll(dir, 0755)
				license := []byte("fixture " + block[1] + " " + block[2])
				files := map[string]string{}
				if block[1] != "enum-iterator" {
					os.WriteFile(filepath.Join(dir, "LICENSE"), license, 0644)
					files["LICENSE"] = fmt.Sprintf("%x", sha256.Sum256(license))
				}
				checksum := block[3]
				if scenario == "changed checksum" && block[1] == "terminal_size" && block[2] == "0.1.17" {
					checksum = strings.Repeat("0", 64)
				}
				data, _ := json.Marshal(map[string]any{"package": checksum, "files": files})
				os.WriteFile(filepath.Join(dir, ".cargo-checksum.json"), data, 0644)
			}
			if scenario == "missing crate" {
				os.RemoveAll(filepath.Join(source, "vendor/hermit-abi-0.1.19"))
			}
			if scenario == "changed license" {
				os.WriteFile(filepath.Join(source, "vendor/terminal_size-0.1.17/LICENSE"), []byte("changed"), 0644)
			}
			stage := gitTrimStageFixture(t, "amd64")
			os.Remove(filepath.Join(stage, "provenance.json"))
			native := fstest.MapFS{}
			for _, p := range gitTrimRuntimeNotices("amd64") {
				native[p] = &fstest.MapFile{Data: []byte("native/runtime full fixture notice")}
			}
			fallback := os.DirFS("../../bundles/toolbox/licenses/git-trim/dependencies")
			if scenario == "missing fallback" {
				fallback = os.DirFS(t.TempDir())
			}
			err := sealGitTrim(toolconfig.GitTrimSpec(), "amd64", source, stage, native, fallback)
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("seal %s: %v", scenario, err)
			}
			if scenario == "valid" {
				for _, n := range []string{"hermit-abi-0.1.19/LICENSE", "hermit-abi-0.2.6/LICENSE", "terminal_size-0.1.17/LICENSE", "terminal_size-0.2.3/LICENSE", "enum-iterator-1.1.3/LICENSE"} {
					if _, err := os.Stat(filepath.Join(stage, "licenses/git-trim/dependencies", n)); err != nil {
						t.Fatal(err)
					}
				}
				r, _ := openRoot(t)
				if err := installGitTrimStage(r, stage, "amd64", toolconfig.GitTrimSpec()); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestGitTrimSmokeOnlyRunsIsolatedDryRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required for the isolated smoke fixture")
	}
	stub := t.TempDir()
	p := filepath.Join(stub, "git-trim")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nset -eu\ntest \"$*\" = '--dry-run --no-update --bases main'\ncase \"$PWD\" in */repo) ;; *) exit 9;; esac\nprintf '%s\\n' fixture-topic\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "../../bundles/toolbox/scripts/git-trim-smoke.sh")
	cmd.Env = append(os.Environ(), "PATH="+stub+":"+os.Getenv("PATH"))
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smoke fixture: %v\n%s", err, b)
	}
	caller := t.TempDir()
	cleanEnv := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			cleanEnv = append(cleanEnv, v)
		}
	}
	git := func(args ...string) []byte {
		t.Helper()
		c := exec.Command("git", args...)
		c.Env = cleanEnv
		b, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("caller fixture: %v\n%s", err, b)
		}
		return b
	}
	git("init", "-b", "main", caller)
	git("-C", caller, "-c", "user.name=Fixture", "-c", "user.email=fixture@bundledock.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "caller fixture")
	beforeConfig, err := os.ReadFile(filepath.Join(caller, ".git/config"))
	if err != nil {
		t.Fatal(err)
	}
	beforeRefs := git("-C", caller, "for-each-ref")
	cmd = exec.Command("sh", "../../bundles/toolbox/scripts/git-trim-smoke.sh")
	cmd.Env = append(cleanEnv, "PATH="+stub+":"+os.Getenv("PATH"), "GIT_DIR="+filepath.Join(caller, ".git"),
		"GIT_WORK_TREE="+caller, "GIT_COMMON_DIR="+filepath.Join(caller, ".git"), "GIT_INDEX_FILE="+filepath.Join(caller, ".git/index"),
		"GIT_OBJECT_DIRECTORY="+filepath.Join(caller, ".git/objects"), "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(caller, ".git/objects"), "GIT_NAMESPACE=caller",
		"GIT_CONFIG="+filepath.Join(caller, ".git/config"), "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.worktree", "GIT_CONFIG_VALUE_0="+caller,
		"GIT_CONFIG_PARAMETERS='core.worktree'='"+caller+"'", "GIT_CONFIG_SYSTEM="+filepath.Join(caller, ".git/config"), "GIT_TEMPLATE_DIR="+caller)
	output, runErr := cmd.CombinedOutput()
	afterConfig, err := os.ReadFile(filepath.Join(caller, ".git/config"))
	if err != nil {
		t.Fatal(err)
	}
	afterRefs := git("-C", caller, "for-each-ref")
	if !bytes.Equal(beforeConfig, afterConfig) || !bytes.Equal(beforeRefs, afterRefs) {
		t.Fatal("inherited Git repository selectors modified the caller fixture")
	}
	if runErr != nil {
		t.Fatalf("isolated hostile-environment smoke: %v\n%s", runErr, output)
	}
}

func TestGitTrimRecipePinsOfflineCompileAndVersionedVendor(t *testing.T) {
	b, err := os.ReadFile("../../bundles/toolbox/scripts/build-git-trim.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, part := range []string{"cargo vendor --locked --versioned-dirs", "cargo build --frozen --offline --release --bin git-trim", "git diff --exit-code HEAD -- .", "--no-tags", "--no-deref HEAD " + toolconfig.GitTrimCommit} {
		if !strings.Contains(s, part) {
			t.Fatalf("fixed recipe missing %s", part)
		}
	}
	b, err = os.ReadFile("../../bundles/toolbox/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	s = string(b)
	if !strings.Contains(s, toolconfig.GitTrimBuilder) || !strings.Contains(s, "RUN --network=none sh /build/build-git-trim.sh compile") {
		t.Fatal("compiler image/offline boundary missing")
	}
}

func gitTrimStageFixture(t *testing.T, arch string) string {
	t.Helper()
	stage := t.TempDir()
	b := make([]byte, 64)
	copy(b, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(b[16:], 2)
	machine := uint16(62)
	if arch == "arm64" {
		machine = 183
	}
	binary.LittleEndian.PutUint16(b[18:], machine)
	binary.LittleEndian.PutUint32(b[20:], 1)
	binary.LittleEndian.PutUint16(b[52:], 64)
	files := map[string][]byte{"bin/git-trim": b, "licenses/git-trim/dependencies/fixture/LICENSE": []byte("fixture license"), "licenses/git-trim/native/fixture-copyright": []byte("native fixture license")}
	receipt := gitTrimReceipt{Spec: toolconfig.GitTrimSpec(), Arch: arch, Files: map[string]string{}}
	for n, b := range files {
		p := filepath.Join(stage, n)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0644); err != nil {
			t.Fatal(err)
		}
		receipt.Files[n] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "provenance.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return stage
}

func TestGitTrimStageIntegrity(t *testing.T) {
	for _, scenario := range []string{"valid", "wrong arch", "changed binary", "symlink", "missing notice", "unrecorded file", "changed source"} {
		t.Run(scenario, func(t *testing.T) {
			stage := gitTrimStageFixture(t, "amd64")
			arch := "amd64"
			switch scenario {
			case "wrong arch":
				arch = "arm64"
			case "changed binary":
				os.WriteFile(filepath.Join(stage, "bin/git-trim"), []byte("changed"), 0755)
			case "symlink":
				os.Remove(filepath.Join(stage, "bin/git-trim"))
				os.Symlink("/etc/passwd", filepath.Join(stage, "bin/git-trim"))
			case "missing notice":
				os.Remove(filepath.Join(stage, "licenses/git-trim/native/fixture-copyright"))
			case "unrecorded file":
				os.WriteFile(filepath.Join(stage, "bin/arbitrary"), []byte("extra"), 0755)
			case "changed source":
				p := filepath.Join(stage, "provenance.json")
				b, _ := os.ReadFile(p)
				b = bytes.ReplaceAll(b, []byte(toolconfig.GitTrimCommit), []byte(strings.Repeat("0", 40)))
				os.WriteFile(p, b, 0644)
			}
			r, dst := openRoot(t)
			err := installGitTrimStage(r, stage, arch, toolconfig.GitTrimSpec())
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("stage result: %v", err)
			}
			if scenario == "valid" {
				info, err := os.Stat(filepath.Join(dst, "bin/git-trim"))
				if err != nil || info.Mode().Perm() != 0755 {
					t.Fatalf("installed binary: %v %v", info, err)
				}
			}
		})
	}
}

// The shell harness remaps only the recipe's container filesystem operations.
// Cargo is a source fixture: it models the independently verified openssl-sys
// 0.9.78 discovery paths without compiling or executing a third-party payload.
func TestGitTrimCompileUsesPrebuiltStaticOpenSSL(t *testing.T) {
	for _, tool := range []string{"sh", "cp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable for compile recipe fixture")
		}
	}
	script, err := filepath.Abs("../../bundles/toolbox/scripts/build-git-trim.sh")
	if err != nil {
		t.Fatal(err)
	}
	for arch, multiarch := range map[string]string{"amd64": "x86_64-linux-gnu", "arm64": "aarch64-linux-gnu"} {
		t.Run(arch, func(t *testing.T) {
			root := t.TempDir()
			stub := filepath.Join(root, "stub")
			if err := os.Mkdir(stub, 0755); err != nil {
				t.Fatal(err)
			}
			write := func(name, data string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(stub, name), []byte("#!/bin/sh\nset -eu\n"+data), 0755); err != nil {
					t.Fatal(err)
				}
			}
			write("rustc", "test \"$*\" = --version\nprintf '%s\\n' 'rustc 1.75.0 (82e1608df 2023-12-21)'\n")
			write("git", "test \"$*\" = 'diff --exit-code HEAD -- .'\n")
			write("cargo", `test "$*" = 'build --frozen --offline --release --bin git-trim'
# With both explicit directories, openssl-sys bypasses its pkg-config exit and
# reaches determine_mode(OPENSSL_STATIC). OPENSSL_STATIC alone is insufficient.
if [ "${OPENSSL_LIB_DIR-}" = "/usr/lib/$FIXTURE_MULTIARCH" ] &&
   [ "${OPENSSL_INCLUDE_DIR-}" = /usr/include ] && [ "${OPENSSL_STATIC-}" = 1 ]; then
    cp "$FIXTURE_ROOT/static" "$FIXTURE_ROOT/compiled"
else
    cp "$FIXTURE_ROOT/dynamic" "$FIXTURE_ROOT/compiled"
fi
`)
			write("tools-bundle", `test "$*" = "--arch $FIXTURE_ARCH --git-trim-mode seal --git-trim-source /src --git-trim-stage /out"
`)
			for name, libs := range map[string][]string{"static": {"libc.so.6", "libgcc_s.so.1"}, "dynamic": {"libssl.so.3", "libcrypto.so.3", "libc.so.6"}} {
				if err := os.WriteFile(filepath.Join(root, name), gitTrimELFFixture(arch, libs), 0644); err != nil {
					t.Fatal(err)
				}
			}
			harness := `
cd() { test "$1" = /src; command cd "$FIXTURE_ROOT"; }
mkdir() { test "$*" = '-p /out/bin'; }
cp() {
    if [ "$*" = '/target/release/git-trim /out/bin/git-trim' ]; then
        command cp "$FIXTURE_ROOT/compiled" "$FIXTURE_ROOT/staged"
    else
        command cp "$@"
    fi
}
set -- compile "$FIXTURE_ARCH"
. "$FIXTURE_SCRIPT"
`
			cmd := exec.Command("sh", "-c", harness)
			env := []string{}
			for _, v := range os.Environ() {
				if !strings.HasPrefix(v, "OPENSSL_") && !strings.HasPrefix(v, "FIXTURE_") && !strings.HasPrefix(v, "PATH=") {
					env = append(env, v)
				}
			}
			cmd.Env = append(env, "PATH="+stub+":"+os.Getenv("PATH"), "OPENSSL_STATIC=1", "FIXTURE_ROOT="+root, "FIXTURE_SCRIPT="+script, "FIXTURE_MULTIARCH="+multiarch, "FIXTURE_ARCH="+arch)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("compile recipe fixture: %v\n%s", err, b)
			}
			b, err := os.ReadFile(filepath.Join(root, "staged"))
			if err != nil {
				t.Fatal(err)
			}
			if err := gitTrimBinary(b, arch); err != nil {
				t.Fatalf("recipe must produce no OpenSSL runtime dependency: %v", err)
			}
		})
	}
}

func TestGitTrimRejectsUnexpectedRuntimeLibraries(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, lib := range []string{"libc.so.6", "libssl.so.3", "libcrypto.so.3", "libunknown.so.1"} {
			t.Run(arch+"/"+lib, func(t *testing.T) {
				b := gitTrimELFFixture(arch, []string{lib})
				err := gitTrimBinary(b, arch)
				if lib == "libc.so.6" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "unexpected runtime dependency "+lib) {
					t.Fatalf("runtime guard: %v", err)
				}
				readelf, err := exec.LookPath("readelf")
				if err != nil {
					t.Skip("readelf unavailable for independent ELF inspection")
				}
				file := filepath.Join(t.TempDir(), "fixture.elf")
				if err := os.WriteFile(file, b, 0644); err != nil {
					t.Fatal(err)
				}
				out, err := exec.Command(readelf, "--dynamic", file).CombinedOutput()
				if err != nil || !strings.Contains(string(out), "(NEEDED)") || !strings.Contains(string(out), "["+lib+"]") {
					t.Fatalf("readelf DT_NEEDED fixture: %v\n%s", err, out)
				}
			})
		}
	}
}

// ELF64 with real SHT_STRTAB/SHT_DYNAMIC sections and PT_DYNAMIC metadata.
// No instructions are present, and these data-only fixtures are never executed.
func gitTrimELFFixture(arch string, libs []string) []byte {
	dynstr := []byte{0}
	dynamic := []byte{}
	tag := func(kind, value uint64) {
		dynamic = binary.LittleEndian.AppendUint64(dynamic, kind)
		dynamic = binary.LittleEndian.AppendUint64(dynamic, value)
	}
	for _, lib := range libs {
		tag(uint64(elf.DT_NEEDED), uint64(len(dynstr)))
		dynstr = append(dynstr, append([]byte(lib), 0)...)
	}
	tag(uint64(elf.DT_NULL), 0)
	names := []byte("\x00.dynstr\x00.dynamic\x00.shstrtab\x00")
	b := make([]byte, 120)
	copy(b, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	put16 := func(offset int, value uint16) { binary.LittleEndian.PutUint16(b[offset:], value) }
	put32 := func(offset int, value uint32) { binary.LittleEndian.PutUint32(b[offset:], value) }
	put64 := func(offset int, value uint64) { binary.LittleEndian.PutUint64(b[offset:], value) }
	put16(16, uint16(elf.ET_DYN))
	put16(18, uint16(elf.EM_X86_64))
	if arch == "arm64" {
		put16(18, uint16(elf.EM_AARCH64))
	}
	put32(20, 1)
	put64(32, 64)
	put16(52, 64)
	put16(54, 56)
	put16(56, 1)
	put16(58, 64)
	put16(60, 4)
	put16(62, 3)
	strOffset := len(b)
	b = append(b, dynstr...)
	dynamicOffset := len(b)
	b = append(b, dynamic...)
	nameOffset := len(b)
	b = append(b, names...)
	sectionOffset := len(b)
	b = append(b, make([]byte, 4*64)...)
	put64(40, uint64(sectionOffset))
	put32(64, uint32(elf.PT_DYNAMIC))
	put64(72, uint64(dynamicOffset))
	put64(96, uint64(len(dynamic)))
	put64(104, uint64(len(dynamic)))
	section := func(index, name int, kind elf.SectionType, offset, size, link, entsize int) {
		base := sectionOffset + index*64
		put32(base, uint32(name))
		put32(base+4, uint32(kind))
		put64(base+24, uint64(offset))
		put64(base+32, uint64(size))
		put32(base+40, uint32(link))
		put64(base+56, uint64(entsize))
	}
	section(1, 1, elf.SHT_STRTAB, strOffset, len(dynstr), 0, 0)
	section(2, 9, elf.SHT_DYNAMIC, dynamicOffset, len(dynamic), 1, 16)
	section(3, 18, elf.SHT_STRTAB, nameOffset, len(names), 0, 0)
	return b
}
