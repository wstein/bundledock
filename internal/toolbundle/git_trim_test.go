package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
