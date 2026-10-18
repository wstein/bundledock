package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSCCArchiveInstallsOnlyExecutable(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["scc"]
	if !ok {
		t.Fatal("scc official native recipe missing")
	}
	if p.Install.Kind != "tar.gz" || p.Install.Members["scc"] != "bin/scc" {
		t.Fatalf("unexpected recipe: %+v", p.Install)
	}
	if len(p.Notices) < 2 {
		t.Fatalf("missing source and full upstream license: %v", p.Notices)
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := archive(t, member{"scc", "synthetic native executable", 0755, 0}, member{"unrequested", "unrequested executable", 0755, 0}, member{"README.md", "must not install or execute", 0644, 0})
	if err := Extract(root, "scc", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/scc")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("unexpected README: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
