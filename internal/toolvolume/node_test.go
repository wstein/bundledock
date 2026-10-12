package toolvolume

import (
	"bundledock/internal/toolbundle"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nodeBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for name, data := range map[string]string{"payload/bin/node": "launcher", "payload/lib/node_modules/npm/bin/npm-cli.js": "script"} {
		if err := toolbundle.WriteFile(r, name, strings.NewReader(data), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := toolbundle.MakeDirectory(r, "payload/bin"); err != nil {
		t.Fatal(err)
	}
	if err := r.Symlink("../lib/node_modules/npm/bin/npm-cli.js", "payload/bin/npm"); err != nil {
		t.Fatal(err)
	}
	if err := toolbundle.WriteFile(r, "payload/lib/node_modules/npm/bin/npx-cli.js", strings.NewReader("npx"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := r.Symlink("../lib/node_modules/npm/bin/npx-cli.js", "payload/bin/npx"); err != nil {
		t.Fatal(err)
	}
	s := toolbundle.Spec{Version: "v24.21.0", URL: "https://nodejs.org/dist/v24.21.0/node-v24.21.0-linux-x64.tar.gz", SHA256: strings.Repeat("a", 64), Verification: "publisher-sha256"}
	if err := toolbundle.WriteNodeManifest(r, "amd64", map[string]toolbundle.Spec{"node": s}); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestNodeVolumeRequiresCallerOptIn(t *testing.T) {
	src := nodeBundle(t)
	dst := t.TempDir()
	if err := Initialize(src, dst, io.Discard); err == nil {
		t.Fatal("self-declared manifest enabled links without caller opt-in")
	}
	absent(t, dst, ".ready")
}
func TestNodeVolumeFidelityAndRepeat(t *testing.T) {
	src := nodeBundle(t)
	dst := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := InitializeWithProfile(src, dst, toolbundle.NodeProfile, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.Readlink(filepath.Join(dst, "payload/bin/npm"))
	if err != nil || got != "../lib/node_modules/npm/bin/npm-cli.js" {
		t.Fatalf("link %q: %v", got, err)
	}
}
func TestNodeVolumeRejectsTampering(t *testing.T) {
	for _, kind := range []string{"retarget", "replace with file", "target contents", "extra link", "missing link", "parent link"} {
		t.Run(kind, func(t *testing.T) {
			src := nodeBundle(t)
			dst := t.TempDir()
			if err := InitializeWithProfile(src, dst, toolbundle.NodeProfile, io.Discard); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dst, "payload/bin/npm")
			switch kind {
			case "retarget":
				os.Remove(link)
				if err := os.Symlink("../lib/node_modules/npm/bin/other.js", link); err != nil {
					t.Fatal(err)
				}
			case "replace with file":
				os.Remove(link)
				if err := os.WriteFile(link, []byte("../lib/node_modules/npm/bin/npm-cli.js"), 0644); err != nil {
					t.Fatal(err)
				}
			case "target contents":
				write(t, dst, "payload/lib/node_modules/npm/bin/npm-cli.js", "tampered")
			case "extra link":
				if err := os.Symlink("../lib/node_modules/npm/bin/npm-cli.js", filepath.Join(dst, "payload/bin/extra")); err != nil {
					t.Fatal(err)
				}
			case "missing link":
				os.Remove(link)
			case "parent link":
				p := filepath.Join(dst, "payload/bin")
				if err := os.Rename(p, p+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(".bin-old", p); err != nil {
					t.Fatal(err)
				}
			}
			if err := InitializeWithProfile(src, dst, toolbundle.NodeProfile, io.Discard); err == nil {
				t.Fatal("accepted tampered graph")
			}
		})
	}
}

func TestNodeRejectsForgedProfileAndProvenance(t *testing.T) {
	for _, kind := range []string{"code-server profile", "wrong URL", "wrong platform"} {
		t.Run(kind, func(t *testing.T) {
			src := nodeBundle(t)
			p := filepath.Join(src, "manifest.json")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			switch kind {
			case "code-server profile":
				s = strings.Replace(s, "node-runtime", "code-server-standalone", 1)
			case "wrong URL":
				s = strings.Replace(s, "nodejs.org", "untrusted.invalid", 1)
			case "wrong platform":
				s = strings.Replace(s, "linux/amd64", "amd64", 1)
			}
			if err := os.WriteFile(p, []byte(s), 0644); err != nil {
				t.Fatal(err)
			}
			if err := InitializeWithProfile(src, t.TempDir(), toolbundle.NodeProfile, io.Discard); err == nil {
				t.Fatal("forged profile accepted")
			}
		})
	}
}
