package testdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewHasNoSymlinkComponents(t *testing.T) {
	dir := New(t)
	if !filepath.IsAbs(dir) {
		t.Fatalf("%s is not absolute", dir)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		t.Fatalf("path %s resolves to %s (%v)", dir, resolved, err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("%s is not a directory: %v", dir, err)
	}
}
