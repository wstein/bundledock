package toolbundle

import (
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestToolboxOpenTofuInstallsExecutableAndNotices(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["opentofu"]
	if !ok {
		t.Fatal("opentofu official native recipe missing")
	}
	dst := testdir.New(t)
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := archive(t, member{"tofu", "synthetic native executable", 0755, 0}, member{"LICENSE", "MPL text", 0644, 0}, member{"README.md", "readme", 0644, 0}, member{"CHANGELOG.md", "changes", 0644, 0}, member{"unrequested", "unrequested", 0755, 0})
	if err := Extract(root, "opentofu", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/tofu")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	for _, name := range []string{"LICENSE", "README.md", "CHANGELOG.md"} {
		if _, err := os.Stat(filepath.Join(dst, "licenses/opentofu", name)); err != nil {
			t.Fatalf("retained %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
