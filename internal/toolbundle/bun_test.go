package toolbundle

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func bunZIP(t *testing.T, root, extra string, mode os.FileMode) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: root + "/bun", Method: zip.Deflate}
	h.SetMode(mode)
	f, e := z.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte("binary"))
	if extra != "" {
		h := &zip.FileHeader{Name: extra}
		h.SetMode(0644)
		f, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte("resource"))
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestBunZIPPreservesFullPayloadAndRejectsHostileGraphs(t *testing.T) {
	for _, arch := range []string{"x64-baseline", "aarch64"} {
		t.Run(arch, func(t *testing.T) {
			root := "bun-linux-" + arch
			r, dir := openRoot(t)
			b := bunZIP(t, root, root+"/LICENSE", 0755)
			if err := extractBunZIP(r, bytes.NewReader(b), int64(len(b)), root); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "payload/LICENSE")); err != nil {
				t.Fatal(err)
			}
			if got, err := os.Readlink(filepath.Join(dir, "payload/bunx")); err != nil || got != "bun" {
				t.Fatal(got, err)
			}
		})
	}
	for _, kind := range []string{"wrong root", "escape", "duplicate", "generated link collision", "symlink", "no executable mode"} {
		t.Run(kind, func(t *testing.T) {
			root := "bun-linux-x64-baseline"
			extra := ""
			mode := os.FileMode(0755)
			switch kind {
			case "wrong root":
				root = "bun-linux-x64"
			case "escape":
				extra = root + "/../outside"
			case "duplicate":
				extra = root + "/bun"
			case "generated link collision":
				extra = root + "/bunx"
			case "symlink":
				mode = os.ModeSymlink | 0777
			case "no executable mode":
				mode = 0644
			}
			r, _ := openRoot(t)
			b := bunZIP(t, root, extra, mode)
			if err := extractBunZIP(r, bytes.NewReader(b), int64(len(b)), "bun-linux-x64-baseline"); err == nil {
				t.Fatal("accepted hostile ZIP")
			}
		})
	}
}
func TestBunLaunchersPreserveArgumentsAndEnvironment(t *testing.T) {
	r, dir := openRoot(t)
	if err := writeBunLaunchers(r); err != nil {
		t.Fatal(err)
	}
	// Shell stand-in only: no Bun executable is run. basename verifies bunx dispatch.
	fake := "#!/bin/sh\nprintf '%s\\n' \"${0##*/}\" \"$HOME\" \"$PATH\" \"$BUN_INSTALL\" \"$BUN_INSTALL_CACHE_DIR\" \"$@\"\n"
	if err := WriteFile(r, "payload/bun", strings.NewReader(fake), 0755); err != nil {
		t.Fatal(err)
	}
	if err := r.Symlink("bun", "payload/bunx"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bun", "bunx"} {
		cmd := exec.Command(filepath.Join(dir, "bin", name), "one two", "literal$argument", "")
		cmd.Env = []string{"HOME=/tmp/bun-home", "PATH=/usr/bin:/bin", "BUN_INSTALL=/tmp/custom-bun", "BUN_INSTALL_CACHE_DIR=/tmp/custom-cache"}
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		want := name + "\n/tmp/bun-home\n/usr/bin:/bin\n/tmp/custom-bun\n/tmp/custom-cache\none two\nliteral$argument\n\n"
		if string(out) != want {
			t.Fatalf("got %q want %q", out, want)
		}
	}
}
