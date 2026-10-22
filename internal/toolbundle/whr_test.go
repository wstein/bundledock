package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWhrArchiveInstallsExecutableAndLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["whr"]
	if !ok {
		t.Fatal("whr official native recipe missing")
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// The official archive nests its files below whr_<version>_linux_<arch>/.
	const dir = "whr_0.1.0_linux_amd64/"
	data := archive(t, member{dir + "whr", "synthetic native executable", 0755, 0}, member{dir + "LICENSE", "license text", 0644, 0}, member{dir + "README.md", "readme", 0644, 0}, member{dir + "third_party/", "", 0755, tar.TypeDir}, member{dir + "third_party/README.md", "third party readme", 0644, 0}, member{dir + "third_party/cobra/LICENSE", "cobra license", 0644, 0}, member{dir + "third_party/x-sys/PATENTS", "x-sys patents", 0644, 0}, member{dir + "unrequested", "unrequested executable", 0755, 0})
	if err := Extract(root, "whr", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/whr")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "licenses/whr/LICENSE")); err != nil || string(got) != "license text" {
		t.Fatalf("license: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "licenses/whr/README.md")); err != nil || string(got) != "readme" {
		t.Fatalf("readme must not be shadowed by third_party/README.md: %q %v", got, err)
	}
	for file, want := range map[string]string{"third_party/README.md": "third party readme", "third_party/cobra/LICENSE": "cobra license", "third_party/x-sys/PATENTS": "x-sys patents"} {
		if got, err := os.ReadFile(filepath.Join(dst, "licenses/whr", file)); err != nil || string(got) != want {
			t.Fatalf("third-party notice %s: %q %v", file, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
