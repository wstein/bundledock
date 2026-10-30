package toolbundle

import (
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestZkArchiveInstallsOnlyExecutable(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["zk"]
	if !ok {
		t.Fatal("zk official native recipe missing")
	}
	dst := testdir.New(t)
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// The official archive is flat: it carries only the zk executable.
	data := archive(t, member{"zk", "synthetic native executable", 0755, 0}, member{"unrequested", "unrequested executable", 0755, 0})
	if err := Extract(root, "zk", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/zk")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
