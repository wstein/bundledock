package toolbundle

import (
	"os"
	"path/filepath"
	"testing"
)

// A symlink's own mtime is part of the layer tar header; the target must stay
// untouched when the link is normalized.
func TestNormalizeMtimesSetsSymlinkItself(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	if err := NormalizeMtimes(root, 1704067200); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.ModTime().Unix() != 1704067200 {
		t.Fatalf("symlink mtime %v err %v", info, err)
	}
}

func TestNormalizeMtimesDoesNotFollowSymlinkOutOfRoot(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(outside)
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := NormalizeMtimes(root, 1704067200); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(outside)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("symlink target outside the root was modified")
	}
}
