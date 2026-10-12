package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTofuArchivesKeepCLIAndNotices(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/opentofu")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["opentofu"]
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			files := []member{{"tofu", "official binary stand-in", 0755, 0}}
			for _, name := range []string{"LICENSE", "README.md", "CHANGELOG.md"} {
				files = append(files, member{name, "unchanged " + name, 0644, 0})
			}
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := Extract(root, "opentofu", p, bytes.NewReader(archive(t, files...))); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"LICENSE", "README.md", "CHANGELOG.md"} {
				data, err := os.ReadFile(filepath.Join(dir, "licenses/opentofu", name))
				if err != nil || string(data) != "unchanged "+name {
					t.Fatal(name, err)
				}
			}
			if err := WriteManifest(root, arch, c.Lock.Platforms[arch]); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "bin/tofu")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "bin/terraform")); !os.IsNotExist(err) {
				t.Fatal("unexpected renamed binary")
			}
		})
	}
	for _, bad := range []member{{"../tofu", "", 0755, 0}, {"tofu", "", 0755, tar.TypeSymlink}, {"tofu", "", 0644, 0}, {"tofu", "", 0755, tar.TypeFifo}} {
		dir := t.TempDir()
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		err = Extract(root, "opentofu", p, bytes.NewReader(archive(t, bad)))
		root.Close()
		if err == nil {
			t.Fatal("unsafe archive accepted", bad)
		}
	}
}
