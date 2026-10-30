package toolbundle

import (
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestUtilityArchivesRetainBatAssets(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			p := c.Catalog.Packages["bat"]
			prefix := "bat-v0.26.1-" + p.Architectures[arch] + "-unknown-linux-musl/"
			files := []member{{prefix + "bat", "synthetic executable", 0755, 0}}
			for _, name := range p.Install.ArchiveNotices {
				entry := prefix + name
				if name == "bat.bash" || name == "bat.fish" || name == "_bat.ps1" || name == "bat.zsh" {
					entry = prefix + "autocomplete/" + name
				}
				files = append(files, member{entry, "upstream " + name, 0644, 0})
			}
			dst := testdir.New(t)
			root, err := os.OpenRoot(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := Extract(root, "bat", p, bytes.NewReader(archive(t, files...))); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"CHANGELOG.md", "README.md", "bat.1", "bat.bash", "bat.fish", "_bat.ps1", "bat.zsh"} {
				b, err := os.ReadFile(filepath.Join(dst, "licenses", "bat", name))
				if err != nil || string(b) != "upstream "+name {
					t.Fatalf("missing unchanged bat asset %s: %v", name, err)
				}
			}
		})
	}
}
