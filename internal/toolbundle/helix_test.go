package toolbundle

import (
	"archive/tar"
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

func helixArchive(t *testing.T, prefix, extra string, kind byte) []byte {
	t.Helper()
	var b bytes.Buffer
	x, err := xz.NewWriter(&b)
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(x)
	for name, data := range map[string]string{"hx": "#!/bin/sh\nprintf '%s\\n' \"$HELIX_RUNTIME\" \"$HOME\" \"$@\"\n", "runtime/grammars/go.so": "compiled grammar", "runtime/queries/go/highlights.scm": "query", "runtime/themes/licenses/theme.LICENSE": "theme notice", "LICENSE": "MPL2"} {
		mode := int64(0644)
		if name == "hx" {
			mode = 0755
		}
		if err := w.WriteHeader(&tar.Header{Name: prefix + "/" + name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if extra != "" {
		if err := w.WriteHeader(&tar.Header{Name: extra, Mode: 0644, Typeflag: kind, Linkname: "hx"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := x.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func helixPackage() toolconfig.Package {
	return toolconfig.Package{Install: toolconfig.Install{Kind: "tar.xz", Profile: "helix-runtime", Preserve: true, Members: map[string]string{"hx": "lib/helix/hx"}}}
}
func TestHelixRelocatedRuntime(t *testing.T) {
	for _, arch := range []string{"x86_64", "aarch64"} {
		t.Run(arch, func(t *testing.T) {
			dst := testdir.New(t)
			r, err := os.OpenRoot(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if err := Extract(r, "helix", helixPackage(), bytes.NewReader(helixArchive(t, "helix-25.07.1-"+arch+"-linux", "", 0))); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"lib/helix/hx", "lib/helix/LICENSE", "lib/helix/runtime/grammars/go.so", "lib/helix/runtime/queries/go/highlights.scm", "lib/helix/runtime/themes/licenses/theme.LICENSE"} {
				if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
					t.Fatal(err)
				}
			}
			// Execute only our synthetic fixture script, never an upstream binary.
			home := testdir.New(t)
			cmd := exec.Command(filepath.Join(dst, "bin/hx"), "file with spaces", "--health")
			cmd.Env = append(os.Environ(), "HOME="+home, "HELIX_RUNTIME=/wrong")
			got, err := cmd.CombinedOutput()
			want := filepath.Join(dst, "lib/helix/runtime") + "\n" + home + "\nfile with spaces\n--health\n"
			if err != nil || string(got) != want {
				t.Fatalf("launcher: %v %q want %q", err, got, want)
			}
			if err := WriteManifest(r, "amd64", map[string]Spec{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestHelixRejectsForeignAndLinkedMembers(t *testing.T) {
	for _, tc := range []struct {
		root, extra string
		kind        byte
	}{{"foreign", "", 0}, {"helix-25.07.1-x86_64-linux", "other/runtime/query", tar.TypeReg}, {"helix-25.07.1-x86_64-linux", "helix-25.07.1-x86_64-linux/runtime/link", tar.TypeSymlink}, {"helix-25.07.1-x86_64-linux", "helix-25.07.1-x86_64-linux/runtime/link", tar.TypeLink}, {"helix-25.07.1-x86_64-linux", "helix-25.07.1-x86_64-linux/../escape", tar.TypeReg}} {
		r, err := os.OpenRoot(testdir.New(t))
		if err != nil {
			t.Fatal(err)
		}
		err = Extract(r, "helix", helixPackage(), bytes.NewReader(helixArchive(t, tc.root, tc.extra, tc.kind)))
		r.Close()
		if err == nil {
			t.Fatalf("accepted %s %s", tc.root, tc.extra)
		}
	}
	p := helixPackage()
	p.Install.Profile = ""
	r, err := os.OpenRoot(testdir.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	err = Extract(r, "helix", p, bytes.NewReader(helixArchive(t, "helix-25.07.1-x86_64-linux", "", 0)))
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("ordinary extractor changed: %v", err)
	}
}

func TestHelixRootMatchesVerifiedPin(t *testing.T) {
	for _, archiveRoot := range []string{"helix-25.07.1-aarch64-linux", "helix-26.01.1-x86_64-linux"} {
		t.Run(archiveRoot, func(t *testing.T) {
			r, err := os.OpenRoot(testdir.New(t))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			err = extract(r, "helix", helixPackage(), bytes.NewReader(helixArchive(t, archiveRoot, "", 0)), "helix-25.07.1-x86_64-linux")
			if err == nil {
				t.Fatal("accepted archive root differing from verified release pin")
			}
		})
	}
}
