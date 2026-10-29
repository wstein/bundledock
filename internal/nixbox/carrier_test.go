package nixbox

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// carrierSource lays out the image tree: store, var/nix and profile.
func carrierSource(t *testing.T) (string, PlatformLock) {
	t.Helper()
	src := t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, pl := fakeStoreIn(t, filepath.Join(src, "store"))
	for rel, body := range map[string]string{"var/nix/db/db.sqlite": "db", "profile/share/nixbox/lock.json": "{}"} {
		full := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(pathA+"/bin/tool", filepath.Join(src, "profile/tool")); err != nil {
		t.Fatal(err)
	}
	// Nix store directories are read-only.
	if err := os.Chmod(filepath.Join(src, "store", filepath.Base(pathA)), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(src, "store", filepath.Base(pathA)), 0o755) })
	return src, pl
}

func TestInitializeCopiesVerifiesAndMarksReady(t *testing.T) {
	src, pl := carrierSource(t)
	dst := t.TempDir()
	var out bytes.Buffer
	if err := Initialize(src, dst, pl, &out); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(dst, "store", filepath.Base(pathA)), 0o755) })
	if !strings.Contains(out.String(), "initialized") {
		t.Errorf("output %q", out.String())
	}
	for _, rel := range []string{".ready", "store/" + filepath.Base(pathA) + "/bin/tool", "var/nix/db/db.sqlite"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, ".initializing")); err == nil {
		t.Error("lock left behind")
	}
	if target, _ := os.Readlink(filepath.Join(dst, "profile/tool")); target != pathA+"/bin/tool" {
		t.Errorf("symlink target %q", target)
	}
	info, _ := os.Stat(filepath.Join(dst, "store", filepath.Base(pathA)))
	if info.Mode().Perm() != 0o555 {
		t.Errorf("store directory mode %v, want 0555", info.Mode().Perm())
	}
	if err := VerifyStore(pl, filepath.Join(dst, "store")); err != nil {
		t.Error(err)
	}
}

func TestInitializeIsIdempotentAndDetectsDrift(t *testing.T) {
	src, pl := carrierSource(t)
	dst := t.TempDir()
	if err := Initialize(src, dst, pl, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(dst, "store", filepath.Base(pathA)), 0o755) })
	if err := Initialize(src, dst, pl, &bytes.Buffer{}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	drift := map[string]func(){
		"added file":   func() { os.WriteFile(filepath.Join(dst, "profile/extra"), nil, 0o644) },
		"changed file": func() { os.WriteFile(filepath.Join(dst, "var/nix/db/db.sqlite"), []byte("DB"), 0o644) },
		"changed link": func() {
			os.Remove(filepath.Join(dst, "profile/tool"))
			os.Symlink("/etc/passwd", filepath.Join(dst, "profile/tool"))
		},
	}
	for name, f := range drift {
		t.Run(name, func(t *testing.T) {
			f()
			if err := Initialize(src, dst, pl, &bytes.Buffer{}); err == nil {
				t.Fatal("accepted a drifted volume")
			}
			if _, err := os.Stat(filepath.Join(dst, ".initializing")); err == nil {
				t.Error("lock left behind after a failure")
			}
		})
	}
}

func TestInitializeRefusals(t *testing.T) {
	src, pl := carrierSource(t)
	t.Run("nonempty volume", func(t *testing.T) {
		dst := t.TempDir()
		os.WriteFile(filepath.Join(dst, "stray"), nil, 0o644)
		if err := Initialize(src, dst, pl, &bytes.Buffer{}); err == nil {
			t.Fatal("initialized a nonempty volume")
		}
	})
	t.Run("running lock", func(t *testing.T) {
		dst := t.TempDir()
		os.Mkdir(filepath.Join(dst, ".initializing"), 0o755)
		if err := Initialize(src, dst, pl, &bytes.Buffer{}); err == nil {
			t.Fatal("ignored another initializer")
		}
		if _, err := os.Stat(filepath.Join(dst, ".initializing")); err != nil {
			t.Error("removed a lock it does not own")
		}
	})
	t.Run("corrupt source store", func(t *testing.T) {
		bad, pl := carrierSource(t)
		os.Chmod(filepath.Join(bad, "store", filepath.Base(pathA)), 0o755)
		os.WriteFile(filepath.Join(bad, "store", filepath.Base(pathA), "bin", "tool"), []byte("tampered"), 0o755)
		dst := t.TempDir()
		if err := Initialize(bad, dst, pl, &bytes.Buffer{}); err == nil {
			t.Fatal("copied a store that fails its lock")
		}
		if _, err := os.Stat(filepath.Join(dst, ".ready")); err == nil {
			t.Error("marked a failed volume ready")
		}
	})
	t.Run("reserved name in source", func(t *testing.T) {
		bad, pl := carrierSource(t)
		os.WriteFile(filepath.Join(bad, ".ready"), nil, 0o644)
		if err := Initialize(bad, t.TempDir(), pl, &bytes.Buffer{}); err == nil {
			t.Fatal("accepted a source with a reserved name")
		}
	})
	t.Run("missing mount", func(t *testing.T) {
		if err := Initialize(src, filepath.Join(t.TempDir(), "absent"), pl, &bytes.Buffer{}); err == nil {
			t.Fatal("no error without a mounted volume")
		}
	})
}
