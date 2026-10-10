package toolvolume

import (
	"bundledock/internal/toolbundle"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for name, data := range map[string]string{"bin/codex": "cli", "bin/codex-code-mode-host": "host", "codex-path/rg": "rg", "codex-resources/voice/lib/libfixture.so": "lib", "codex-package.json": "{}"} {
		mode := os.FileMode(0755)
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".so") {
			mode = 0644
		}
		if err := toolbundle.WriteFile(root, name, strings.NewReader(data), mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := toolbundle.MakeDirectory(root, "empty/nested"); err != nil {
		t.Fatal(err)
	}
	if err := toolbundle.WriteManifest(root, "amd64", map[string]toolbundle.Spec{"codex": {Version: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	return dir
}
func write(t *testing.T, dir, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
func absent(t *testing.T, dir, name string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		t.Fatalf("expected absent %s, got %v", name, err)
	}
}
func TestInitializeAndRepeat(t *testing.T) {
	src := bundle(t)
	dst := t.TempDir()
	var out bytes.Buffer
	if err := Initialize(src, dst, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Tool volume initialized\n" {
		t.Fatalf("output %q", out.String())
	}
	for _, name := range []string{"bin/codex", "codex-resources/voice/lib/libfixture.so", "manifest.json", "SHA256SUMS"} {
		a, _ := os.ReadFile(filepath.Join(src, name))
		b, _ := os.ReadFile(filepath.Join(dst, name))
		if !bytes.Equal(a, b) {
			t.Fatalf("copy mismatch %s", name)
		}
		aInfo, _ := os.Stat(filepath.Join(src, name))
		bInfo, _ := os.Stat(filepath.Join(dst, name))
		if aInfo.Mode().Perm() != bInfo.Mode().Perm() {
			t.Fatalf("mode mismatch %s", name)
		}
	}
	if info, err := os.Stat(filepath.Join(dst, "empty/nested")); err != nil || !info.IsDir() {
		t.Fatalf("empty directory missing %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".ready")); err != nil {
		t.Fatal(err)
	}
	absent(t, dst, ".initializing")
	out.Reset()
	if err := Initialize(src, dst, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("repeat initialized again")
	}
}
func TestRejectInvalidVolumes(t *testing.T) {
	for _, scenario := range []string{"contents", "manifest", "sums", "missing helper", "missing ready", "nonempty", "locked", "ready symlink", "file symlink", "parent symlink"} {
		t.Run(scenario, func(t *testing.T) {
			src := bundle(t)
			dst := t.TempDir()
			if scenario != "nonempty" && scenario != "locked" {
				if err := Initialize(src, dst, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "contents":
				write(t, dst, "bin/codex", "corrupt")
			case "manifest":
				write(t, dst, "manifest.json", "corrupt")
			case "sums":
				write(t, dst, "SHA256SUMS", "corrupt")
			case "missing helper":
				if err := os.Remove(filepath.Join(dst, "bin/codex-code-mode-host")); err != nil {
					t.Fatal(err)
				}
			case "missing ready":
				if err := os.Remove(filepath.Join(dst, ".ready")); err != nil {
					t.Fatal(err)
				}
			case "nonempty":
				write(t, dst, "partial", "leftover")
			case "locked":
				if err := os.Mkdir(filepath.Join(dst, ".initializing"), 0755); err != nil {
					t.Fatal(err)
				}
			case "ready symlink":
				os.Remove(filepath.Join(dst, ".ready"))
				if err := os.Symlink(filepath.Join(src, "manifest.json"), filepath.Join(dst, ".ready")); err != nil {
					t.Fatal(err)
				}
			case "file symlink":
				os.Remove(filepath.Join(dst, "bin/codex"))
				if err := os.Symlink(filepath.Join(src, "bin/codex"), filepath.Join(dst, "bin/codex")); err != nil {
					t.Fatal(err)
				}
			case "parent symlink":
				if err := os.RemoveAll(filepath.Join(dst, "bin")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(src, "bin"), filepath.Join(dst, "bin")); err != nil {
					t.Fatal(err)
				}
			}
			if err := Initialize(src, dst, io.Discard); err == nil {
				t.Fatal("accepted invalid volume")
			}
			if scenario == "locked" {
				if _, err := os.Stat(filepath.Join(dst, ".initializing")); err != nil {
					t.Fatal("deleted existing lock")
				}
			} else {
				absent(t, dst, ".initializing")
			}
		})
	}
	if err := Initialize(bundle(t), filepath.Join(t.TempDir(), "missing"), io.Discard); err == nil {
		t.Fatal("accepted missing volume mount")
	}
}
func TestPartialCopyLeavesNoReadyAndRejectsRetry(t *testing.T) {
	src := bundle(t)
	dst := t.TempDir()
	fail := errors.New("copy interrupted")
	err := initialize(src, dst, io.Discard, func(_, target *os.Root) error {
		if err := toolbundle.WriteFile(target, "partial", strings.NewReader("partial"), 0644); err != nil {
			return err
		}
		return fail
	})
	if !errors.Is(err, fail) {
		t.Fatalf("error %v", err)
	}
	absent(t, dst, ".ready")
	absent(t, dst, ".initializing")
	if err := Initialize(src, dst, io.Discard); err == nil || !strings.Contains(err.Error(), "nonempty") {
		t.Fatalf("retry error %v", err)
	}
}
func TestCopyValidationBeforeReady(t *testing.T) {
	src := bundle(t)
	dst := t.TempDir()
	write(t, src, "bin/codex", "corrupt")
	if err := Initialize(src, dst, io.Discard); err == nil {
		t.Fatal("accepted corrupt source")
	}
	absent(t, dst, ".ready")
	absent(t, dst, ".initializing")
}
func TestConcurrentInitialization(t *testing.T) {
	src := bundle(t)
	dst := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- initialize(src, dst, io.Discard, func(a, b *os.Root) error { close(started); <-release; return copyBundle(a, b) })
	}()
	<-started
	err := Initialize(src, dst, io.Discard)
	close(release)
	firstErr := <-done
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("concurrent initializer error %v", err)
	}
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	if err := Initialize(src, dst, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatRejectsTreeAndPermissionChanges(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{"unexpected executable", func(t *testing.T, dst string) {
			write(t, dst, "bin/git", "shadow system git")
			if err := os.Chmod(filepath.Join(dst, "bin/git"), 0755); err != nil {
				t.Fatal(err)
			}
		}},
		{"unexpected directory", func(t *testing.T, dst string) {
			if err := os.Mkdir(filepath.Join(dst, "injected"), 0755); err != nil {
				t.Fatal(err)
			}
		}},
		{"unexpected symlink", func(t *testing.T, dst string) {
			if err := os.Symlink("codex", filepath.Join(dst, "bin/git")); err != nil {
				t.Fatal(err)
			}
		}},
		{"generic lock is unexpected", func(t *testing.T, dst string) {
			if err := os.Mkdir(filepath.Join(dst, ".lock"), 0755); err != nil {
				t.Fatal(err)
			}
		}},
		{"file executable mode", func(t *testing.T, dst string) {
			if err := os.Chmod(filepath.Join(dst, "bin/codex"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"resource executable mode", func(t *testing.T, dst string) {
			if err := os.Chmod(filepath.Join(dst, "codex-package.json"), 0755); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory mode", func(t *testing.T, dst string) {
			if err := os.Chmod(filepath.Join(dst, "bin"), 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing empty directory", func(t *testing.T, dst string) {
			if err := os.Remove(filepath.Join(dst, "empty/nested")); err != nil {
				t.Fatal(err)
			}
		}},
		{"ready contents", func(t *testing.T, dst string) { write(t, dst, ".ready", "unexpected contents") }},
		{"ready executable mode", func(t *testing.T, dst string) {
			if err := os.Chmod(filepath.Join(dst, ".ready"), 0755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, dst := bundle(t), t.TempDir()
			if err := Initialize(src, dst, io.Discard); err != nil {
				t.Fatal(err)
			}
			c.mutate(t, dst)
			if err := Initialize(src, dst, io.Discard); err == nil {
				t.Fatal("accepted modified volume tree or permissions")
			}
			absent(t, dst, ".initializing")
		})
	}
}

func TestCopyTreeValidationBeforeReady(t *testing.T) {
	for _, change := range []string{"extra executable", "empty directory removed", "executable mode", "lock contents"} {
		t.Run(change, func(t *testing.T) {
			src, dst := bundle(t), t.TempDir()
			err := initialize(src, dst, io.Discard, func(a, b *os.Root) error {
				if err := copyBundle(a, b); err != nil {
					return err
				}
				switch change {
				case "extra executable":
					return toolbundle.WriteFile(b, "bin/git", strings.NewReader("shadow"), 0755)
				case "empty directory removed":
					return b.Remove("empty/nested")
				case "executable mode":
					return os.Chmod(filepath.Join(dst, "bin/codex"), 0644)
				case "lock contents":
					return toolbundle.WriteFile(b, ".initializing/extra", strings.NewReader("unexpected"), 0644)
				}
				return nil
			})
			if err == nil {
				t.Fatal("marked altered copy ready")
			}
			absent(t, dst, ".ready")
			if change != "lock contents" {
				absent(t, dst, ".initializing")
			}
		})
	}
}

func TestRootMountPermissionsMayDiffer(t *testing.T) {
	src, dst := bundle(t), t.TempDir()
	if err := os.Chmod(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(src, dst, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(src, dst, io.Discard); err != nil {
		t.Fatal(err)
	}
}
