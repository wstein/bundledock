package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMisspellArchiveInstallsExecutableAndLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["misspell"]
	if !ok {
		t.Fatal("misspell official native recipe missing")
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// The official archive is wrapped in one directory (goreleaser wrap_in_directory)
	// and carries the executable and LICENSE.
	data := archive(t, member{"misspell_0.8.0_linux_amd64/misspell", "synthetic native executable", 0755, 0}, member{"misspell_0.8.0_linux_amd64/LICENSE", "synthetic MIT license", 0644, 0}, member{"misspell_0.8.0_linux_amd64/unrequested", "unrequested executable", 0755, 0})
	if err := Extract(root, "misspell", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/misspell")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "licenses/misspell/LICENSE")); err != nil || string(got) != "synthetic MIT license" {
		t.Fatalf("license: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
