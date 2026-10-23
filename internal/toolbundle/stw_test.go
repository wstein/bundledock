package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStwArchiveInstallsExecutableAndLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["stw"]
	if !ok {
		t.Fatal("stw official native recipe missing")
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// The official archive nests its files below stw_<version>_linux_<arch>/.
	const dir = "stw_0.1.0_linux_amd64/"
	data := archive(t, member{dir + "stw", "synthetic native executable", 0755, 0}, member{dir + "LICENSE", "license text", 0644, 0}, member{dir + "README.md", "readme", 0644, 0}, member{dir + "third_party/", "", 0755, tar.TypeDir}, member{dir + "third_party/README.md", "third party readme", 0644, 0}, member{dir + "third_party/licenses/MIT.txt", "pflag license", 0644, 0}, member{dir + "third_party/licenses/Go-PATENTS.txt", "yaml patents", 0644, 0}, member{dir + "unrequested", "unrequested executable", 0755, 0})
	if err := Extract(root, "stw", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin/stw")); err != nil || string(got) != "synthetic native executable" {
		t.Fatalf("executable: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "licenses/stw/LICENSE")); err != nil || string(got) != "license text" {
		t.Fatalf("license: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "licenses/stw/README.md")); err != nil || string(got) != "readme" {
		t.Fatalf("readme must not be shadowed by third_party/README.md: %q %v", got, err)
	}
	for file, want := range map[string]string{"third_party/README.md": "third party readme", "third_party/licenses/MIT.txt": "pflag license", "third_party/licenses/Go-PATENTS.txt": "yaml patents"} {
		if got, err := os.ReadFile(filepath.Join(dst, "licenses/stw", file)); err != nil || string(got) != want {
			t.Fatalf("third-party notice %s: %q %v", file, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "bin/unrequested")); !os.IsNotExist(err) {
		t.Fatalf("unexpected unrequested: %v", err)
	}
}
