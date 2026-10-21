package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSmillArchiveInstallsExecutableAndLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["smill"]
	if !ok {
		t.Fatal("smill official native recipe missing")
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// The official archive nests its files below smill_<version>_linux_<arch>/.
	const dir = "smill_0.1.1_linux_amd64/"
	data := archive(t, member{dir + "smill", "synthetic native executable", 0755, 0}, member{dir + "LICENSE", "EUPL text", 0644, 0}, member{dir + "README.md", "readme", 0644, 0}, member{dir + "unrequested", "unrequested executable", 0755, 0})
	if err := Extract(root, "smill", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/smill")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "licenses/smill/LICENSE")); err != nil || string(got) != "EUPL text" {
		t.Fatalf("license: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
