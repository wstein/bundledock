package toolbundle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeMtimesSetsEveryEntry(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	const epoch = int64(1704067200)
	if err := NormalizeMtimes(root, epoch); err != nil {
		t.Fatal(err)
	}
	want := time.Unix(epoch, 0)
	for _, p := range []string{root, filepath.Join(root, "a"), dir, file} {
		info, err := os.Lstat(p)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(want) {
			t.Errorf("%s: mtime %v, want %v", p, info.ModTime(), want)
		}
	}
}

func TestNormalizeMtimesKeepsContentAndMode(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "tool")
	if err := os.WriteFile(file, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NormalizeMtimes(root, 1704067200); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(file)
	data, _ := os.ReadFile(file)
	if info.Mode().Perm() != 0o755 || string(data) != "binary" {
		t.Fatalf("mode %v content %q changed", info.Mode(), data)
	}
}

func TestNormalizeMtimesRejectsBadInput(t *testing.T) {
	if err := NormalizeMtimes(filepath.Join(t.TempDir(), "missing"), 1); err == nil {
		t.Error("missing root accepted")
	}
	if err := NormalizeMtimes(t.TempDir(), -1); err == nil {
		t.Error("negative epoch accepted")
	}
}

// An unreadable directory must fail the walk, not be skipped: a skipped
// subtree would keep its build-time mtimes in the layer.
func TestNormalizeMtimesReportsWalkErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every directory")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(filepath.Join(locked, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	if err := NormalizeMtimes(root, 1); err == nil {
		t.Fatal("unreadable directory was skipped without an error")
	}
}
