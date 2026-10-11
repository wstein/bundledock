package toolvolume

import (
	"bundledock/internal/toolbundle"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func standaloneBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for name, data := range map[string]string{"payload/bin/code-server": "launcher", "payload/lib/node": "node", "payload/node_modules/semver/bin/semver.js": "script"} {
		if err := toolbundle.WriteFile(r, name, strings.NewReader(data), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := toolbundle.MakeDirectory(r, "payload/node_modules/.bin"); err != nil {
		t.Fatal(err)
	}
	if err := r.Symlink("../semver/bin/semver.js", "payload/node_modules/.bin/semver"); err != nil {
		t.Fatal(err)
	}
	s := toolbundle.Spec{Version: "v4.141.0", URL: "https://github.com/coder/code-server/releases/download/v4.141.0/code-server-4.141.0-linux-amd64.tar.gz", SHA256: strings.Repeat("a", 64), Verification: "publisher-sha256"}
	if err := toolbundle.WriteStandaloneManifest(r, "amd64", map[string]toolbundle.Spec{"code-server": s}); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestStandaloneVolumeRequiresCallerOptIn(t *testing.T) {
	src := standaloneBundle(t)
	dst := t.TempDir()
	if err := Initialize(src, dst, io.Discard); err == nil {
		t.Fatal("self-declared manifest enabled links without caller opt-in")
	}
	absent(t, dst, ".ready")
}
func TestStandaloneVolumeFidelityAndRepeat(t *testing.T) {
	src := standaloneBundle(t)
	dst := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := InitializeWithProfile(src, dst, toolbundle.StandaloneProfile, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.Readlink(filepath.Join(dst, "payload/node_modules/.bin/semver"))
	if err != nil || got != "../semver/bin/semver.js" {
		t.Fatalf("link %q: %v", got, err)
	}
}
func TestStandaloneVolumeRejectsTampering(t *testing.T) {
	for _, kind := range []string{"retarget", "replace with file", "target contents", "extra link", "missing link", "parent link"} {
		t.Run(kind, func(t *testing.T) {
			src := standaloneBundle(t)
			dst := t.TempDir()
			if err := InitializeWithProfile(src, dst, toolbundle.StandaloneProfile, io.Discard); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dst, "payload/node_modules/.bin/semver")
			switch kind {
			case "retarget":
				os.Remove(link)
				if err := os.Symlink("../semver/bin/other.js", link); err != nil {
					t.Fatal(err)
				}
			case "replace with file":
				os.Remove(link)
				if err := os.WriteFile(link, []byte("../semver/bin/semver.js"), 0644); err != nil {
					t.Fatal(err)
				}
			case "target contents":
				write(t, dst, "payload/node_modules/semver/bin/semver.js", "tampered")
			case "extra link":
				if err := os.Symlink("../semver/bin/semver.js", filepath.Join(dst, "payload/node_modules/.bin/extra")); err != nil {
					t.Fatal(err)
				}
			case "missing link":
				os.Remove(link)
			case "parent link":
				p := filepath.Join(dst, "payload/node_modules/.bin")
				if err := os.Rename(p, p+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(".bin-old", p); err != nil {
					t.Fatal(err)
				}
			}
			if err := InitializeWithProfile(src, dst, toolbundle.StandaloneProfile, io.Discard); err == nil {
				t.Fatal("accepted tampered graph")
			}
		})
	}
}
