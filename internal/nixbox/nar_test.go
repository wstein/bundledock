package nixbox

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// writeFixture builds the tree whose NAR hash `nix hash path --sri` printed in
// nixos/nix 2.35.4 (see the expected values below).
func writeFixture(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"bin", "e", "sub/deep"} {
		must(os.MkdirAll(filepath.Join(d, dir), 0o755))
	}
	must(os.WriteFile(filepath.Join(d, "a"), []byte("hello\n"), 0o644))
	must(os.WriteFile(filepath.Join(d, "bin/x"), []byte("#!/bin/sh\n"), 0o755))
	must(os.WriteFile(filepath.Join(d, "z"), nil, 0o644))
	must(os.WriteFile(filepath.Join(d, "B"), []byte("x"), 0o644))
	must(os.WriteFile(filepath.Join(d, "sub/deep/ten"), []byte("0123456789"), 0o644))
	must(os.Symlink("a", filepath.Join(d, "l")))
	must(os.Symlink("/nix/store/abc-x/bin", filepath.Join(d, "sub/abs")))
	return d
}

func sri(sum []byte) string { return "sha256-" + base64.StdEncoding.EncodeToString(sum) }

func TestNARHashMatchesNix(t *testing.T) {
	d := writeFixture(t)
	cases := map[string]string{
		".": "sha256-TBNda7Z+9/uLzjSItcY0QcE8WuRzN+33uENG39GScW0=",
		"a": "sha256-HDfQGvQL4ugGkd48w99EN3ppmvuxfGjwgJZLL9Bx/BM=",
		"l": "sha256-stRxoI0wZi8UwK4ecYsW+fwfON5CX0fMoEN+npO8HyQ=",
	}
	for rel, want := range cases {
		sum, _, err := NARHash(filepath.Join(d, rel))
		if err != nil {
			t.Fatal(err)
		}
		if got := sri(sum); got != want {
			t.Errorf("%s: %s, want %s", rel, got, want)
		}
	}
}

func TestNARHashSeesModeAndContentChanges(t *testing.T) {
	d := writeFixture(t)
	base, size, _ := NARHash(d)
	if size == 0 {
		t.Fatal("no size")
	}
	changes := map[string]func(){
		"exec bit":  func() { os.Chmod(filepath.Join(d, "a"), 0o755) },
		"content":   func() { os.WriteFile(filepath.Join(d, "a"), []byte("hellO\n"), 0o644) },
		"extra":     func() { os.WriteFile(filepath.Join(d, "extra"), nil, 0o644) },
		"link":      func() { os.Remove(filepath.Join(d, "l")); os.Symlink("z", filepath.Join(d, "l")) },
		"empty dir": func() { os.Remove(filepath.Join(d, "e")) },
	}
	for name, change := range changes {
		change()
		sum, _, _ := NARHash(d)
		if string(sum) == string(base) {
			t.Errorf("%s did not change the hash", name)
		}
		d2 := writeFixture(t)
		d = d2
	}
}

func TestNARHashTreatsHardLinksAsRegularFilesAndRejectsFifos(t *testing.T) {
	d := writeFixture(t)
	if err := os.Link(filepath.Join(d, "a"), filepath.Join(d, "hard")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NARHash(d); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(d, "fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NARHash(d); err == nil {
		t.Fatal("accepted a fifo")
	}
}
