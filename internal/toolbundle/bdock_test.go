package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBdockArchiveInstallsExecutableAndNotices(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["bdock"]
	if !ok {
		t.Fatal("bdock official native recipe missing")
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// The official archive is flat: bdock, LICENSE and PROVENANCE at the top level.
	data := archive(t, member{"bdock", "synthetic native executable", 0755, 0}, member{"LICENSE", "EUPL text", 0644, 0}, member{"PROVENANCE", "build record", 0644, 0}, member{"unrequested", "unrequested executable", 0755, 0})
	if err := Extract(root, "bdock", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"bin/bdock": "synthetic native executable", "licenses/bdock/LICENSE": "EUPL text", "licenses/bdock/PROVENANCE": "build record"} {
		if got, err := os.ReadFile(filepath.Join(dst, file)); err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", file, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
