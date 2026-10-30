package quay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func initRoot(t *testing.T, doc string) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.MkdirAll(filepath.Join(root, "bundles", "bun"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "bun.md"), []byte(doc), 0o644)
	return root
}

func TestInitDescriptionFromFrontMatter(t *testing.T) {
	root := initRoot(t, "---\ntitle: Bun\ndescription: \"Bun runtime and bunx.\"\n---\n\n# Bun\n\nOther text.\n")
	b, _ := FindBundle("bun")
	path, n, err := InitDescription(root, b)
	if err != nil || n == 0 {
		t.Fatalf("err = %v", err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, want := range []string{"# BundleDock bun", "Bun runtime and bunx.", "https://wstein.github.io/bundledock/", "https://github.com/wstein/bundledock"} {
		if !strings.Contains(text, want) {
			t.Errorf("draft lacks %q: %s", want, text)
		}
	}
	if strings.Contains(text, "Other text") {
		t.Error("used the body despite a front matter description")
	}
	// The draft is accepted by the loader that describe uses.
	if got, ok, err := LoadDescription(root, "bun"); !ok || err != nil || len(got) == 0 {
		t.Fatalf("draft not loadable: %v %v", ok, err)
	}
}

func TestInitDescriptionFromFirstParagraph(t *testing.T) {
	root := initRoot(t, "# Bun\n\n[Back to BundleDock](../README.md)\n\n<!-- note -->\nFirst line\nsecond line.\n\nNext paragraph.\n")
	b, _ := FindBundle("bun")
	path, _, err := InitDescription(root, b)
	raw, _ := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "First line second line.") || strings.Contains(string(raw), "Next paragraph") {
		t.Fatalf("err = %v draft = %s", err, raw)
	}
}

func TestInitDescriptionNeverOverwrites(t *testing.T) {
	root := initRoot(t, "---\ndescription: x\n---\n")
	os.WriteFile(DescriptionPath(root, "bun"), []byte("mine"), 0o644)
	b, _ := FindBundle("bun")
	if _, _, err := InitDescription(root, b); err == nil {
		t.Fatal("overwrote an existing file")
	}
	if raw, _ := os.ReadFile(DescriptionPath(root, "bun")); string(raw) != "mine" {
		t.Fatalf("file changed: %s", raw)
	}
}

func TestInitDescriptionStaysBelowLimit(t *testing.T) {
	root := initRoot(t, "---\ndescription: "+strings.Repeat("é", 9000)+"\n---\n")
	b, _ := FindBundle("bun")
	path, n, err := InitDescription(root, b)
	if err != nil || n > MaxDescription {
		t.Fatalf("n = %d err = %v", n, err)
	}
	if _, ok, err := LoadDescription(root, "bun"); !ok || err != nil {
		t.Fatalf("not loadable: %v %v (%s)", ok, err, path)
	}
}

func TestInitDescriptionMissingDoc(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bundles", "bun"), 0o755)
	b, _ := FindBundle("bun")
	if _, _, err := InitDescription(root, b); err == nil {
		t.Fatal("missing docs accepted")
	}
	if _, err := os.Stat(DescriptionPath(root, "bun")); err == nil {
		t.Fatal("file written despite error")
	}
}
