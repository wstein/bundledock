package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCscribeArchiveInstallsExecutableAndLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["cscribe"]
	if !ok {
		t.Fatal("cscribe official native recipe missing")
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// The official archive nests its files below cscribe_<version>_linux_<arch>/.
	const dir = "cscribe_0.2.0_linux_amd64/"
	data := archive(t, member{dir + "cscribe", "synthetic native executable", 0755, 0}, member{dir + "LICENSE", "EUPL text", 0644, 0}, member{dir + "README.md", "readme", 0644, 0}, member{dir + "unrequested", "unrequested executable", 0755, 0})
	if err := Extract(root, "cscribe", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/cscribe")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "licenses/cscribe/LICENSE")); err != nil || string(got) != "EUPL text" {
		t.Fatalf("license: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
