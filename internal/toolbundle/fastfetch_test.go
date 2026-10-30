package toolbundle

import (
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFastfetchArchiveInstallsOnlyExecutable(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["fastfetch"]
	if !ok {
		t.Fatal("fastfetch official native recipe missing")
	}
	if p.Install.Kind != "tar.gz" || p.Install.Members["fastfetch"] != "bin/fastfetch" {
		t.Fatalf("unexpected recipe: %+v", p.Install)
	}
	if len(p.Notices) < 2 {
		t.Fatalf("missing source and full upstream license: %v", p.Notices)
	}
	dst := testdir.New(t)
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := archive(t,
		member{"fastfetch-linux-amd64/usr/bin/fastfetch", "synthetic native executable", 0755, 0},
		member{"fastfetch-linux-amd64/usr/bin/flashfetch", "unrequested executable", 0755, 0},
		member{"usr/share/fastfetch/presets/example.jsonc", "must not install or execute", 0644, 0},
		member{"fastfetch-linux-amd64/usr/share/bash-completion/completions/fastfetch", "must not collide with binary", 0644, 0},
	)
	if err := Extract(root, "fastfetch", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/fastfetch")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "usr/share/fastfetch/presets/example.jsonc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected preset: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/flashfetch")); !os.IsNotExist(err) {
		t.Fatalf("unexpected flashfetch: %v", err)
	}
}

func TestFastfetchArchiveRejectsUnexecutableOnly(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["fastfetch"]
	dst := testdir.New(t)
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := archive(t,
		member{"fastfetch-linux-amd64/usr/bin/flashfetch", "unrequested executable", 0755, 0},
		member{"fastfetch-linux-amd64/usr/share/bash-completion/completions/fastfetch", "non-executable only", 0644, 0},
	)
	err = Extract(root, "fastfetch", p, bytes.NewReader(data))
	if err == nil || err.Error() != "fastfetch entrypoint is not executable" {
		t.Fatalf("expected unexecutable entrypoint error, got: %v", err)
	}
}

func TestFastfetchArchiveRejectsDuplicateExecutable(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["fastfetch"]
	dst := testdir.New(t)
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := archive(t,
		member{"fastfetch-linux-amd64/usr/bin/fastfetch", "executable 1", 0755, 0},
		member{"fastfetch-linux-amd64/usr/local/bin/fastfetch", "executable 2", 0755, 0},
	)
	err = Extract(root, "fastfetch", p, bytes.NewReader(data))
	if err == nil || err.Error() != `duplicate archive selection "fastfetch"` {
		t.Fatalf("expected duplicate archive selection error, got: %v", err)
	}
}
