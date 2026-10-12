package toolvolume

import (
	"bundledock/internal/toolbundle"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bunBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for n, d := range map[string]string{"payload/bun": "binary", "bin/bun": "launcher", "bin/bunx": "launcher"} {
		if err := toolbundle.WriteFile(r, n, strings.NewReader(d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Symlink("bun", "payload/bunx"); err != nil {
		t.Fatal(err)
	}
	s := toolbundle.Spec{Version: "bun-v1.4.3", URL: "https://github.com/oven-sh/bun/releases/download/bun-v1.4.3/bun-linux-x64-baseline.zip", SHA256: strings.Repeat("a", 64), Verification: "publisher-sha256"}
	if err := toolbundle.WriteBunManifest(r, "amd64", map[string]toolbundle.Spec{"bun": s}); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestBunVolumeRequiresOptInAndRepeats(t *testing.T) {
	src := bunBundle(t)
	dst := t.TempDir()
	if err := Initialize(src, dst, io.Discard); err == nil {
		t.Fatal("self-declared profile accepted")
	}
	absent(t, dst, ".ready")
	for i := 0; i < 2; i++ {
		if err := InitializeWithProfile(src, dst, toolbundle.BunProfile, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if target, err := os.Readlink(filepath.Join(dst, "payload/bunx")); err != nil || target != "bun" {
		t.Fatal(target, err)
	}
}
func TestBunVolumeRejectsTampering(t *testing.T) {
	for _, kind := range []string{"retarget", "replace", "target contents", "target mode", "extra link", "missing link", "parent link", "extra file"} {
		t.Run(kind, func(t *testing.T) {
			src := bunBundle(t)
			dst := t.TempDir()
			if err := InitializeWithProfile(src, dst, toolbundle.BunProfile, io.Discard); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dst, "payload/bunx")
			switch kind {
			case "retarget":
				os.Remove(link)
				os.Symlink("../bin/bun", link)
			case "replace":
				os.Remove(link)
				os.WriteFile(link, []byte("bun"), 0644)
			case "target contents":
				write(t, dst, "payload/bun", "tampered")
			case "target mode":
				os.Chmod(filepath.Join(dst, "payload/bun"), 0644)
			case "extra link":
				os.Symlink("bun", filepath.Join(dst, "payload/extra"))
			case "missing link":
				os.Remove(link)
			case "parent link":
				p := filepath.Join(dst, "payload")
				os.Rename(p, p+"-old")
				os.Symlink("payload-old", p)
			case "extra file":
				write(t, dst, "extra", "extra")
			}
			if err := InitializeWithProfile(src, dst, toolbundle.BunProfile, io.Discard); err == nil {
				t.Fatal("accepted tampered graph")
			}
		})
	}
}
func TestBunVolumeRejectsForgedProvenanceBeforeReady(t *testing.T) {
	for _, kind := range []string{"profile", "url", "version", "platform", "link"} {
		t.Run(kind, func(t *testing.T) {
			src := bunBundle(t)
			p := filepath.Join(src, "manifest.json")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			switch kind {
			case "profile":
				s = strings.Replace(s, "bun-runtime", "node-runtime", 1)
			case "url":
				s = strings.Replace(s, "x64-baseline.zip", "x64.zip", 1)
			case "version":
				s = strings.Replace(s, "bun-v1.4.3", "bun-v1.4.4", 1)
			case "platform":
				s = strings.Replace(s, "linux/amd64", "amd64", 1)
			case "link":
				s = strings.Replace(s, `"payload/bunx": "bun"`, `"payload/bunx": "../bin/bun"`, 1)
			}
			if err := os.WriteFile(p, []byte(s), 0644); err != nil {
				t.Fatal(err)
			}
			dst := t.TempDir()
			if err := InitializeWithProfile(src, dst, toolbundle.BunProfile, io.Discard); err == nil {
				t.Fatal("forged provenance accepted")
			}
			absent(t, dst, ".ready")
		})
	}
}
