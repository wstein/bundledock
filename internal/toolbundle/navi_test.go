package toolbundle

import (
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNaviArchiveInstallsOnlyExecutableAndRetainedLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["navi"]
	if !ok {
		t.Fatal("navi official native recipe missing")
	}
	if p.Install.Kind != "tar.gz" || p.Install.Members["navi"] != "bin/navi" {
		t.Fatalf("unexpected recipe: %+v", p.Install)
	}
	if len(p.Notices) != 2 {
		t.Fatalf("missing source and full upstream license: %v", p.Notices)
	}
	dst := testdir.New(t)
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := archive(t, member{"navi", "synthetic native executable", 0755, 0}, member{"example.cheat", "must not install or execute", 0644, 0})
	if err := Extract(root, "navi", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/navi")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "example.cheat")); !os.IsNotExist(err) {
		t.Fatalf("unexpected cheatsheet: %v", err)
	}
}
