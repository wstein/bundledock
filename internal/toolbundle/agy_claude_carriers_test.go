package toolbundle

import (
	"bundledock/internal/toolconfig"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// These fixtures exercise singleton download, integrity, notice copying and
// manifest creation without executing upstream software.
func TestAgyClaudeStandaloneCarriers(t *testing.T) {
	payloads, specs, server := fixture(t)
	for _, name := range []string{"agy", "claude"} {
		for _, arch := range []string{"amd64", "arm64"} {
			t.Run(name+"/"+arch, func(t *testing.T) {
				directory := "../../bundles/" + name
				c, err := toolconfig.Load(directory)
				if err != nil {
					t.Fatal(err)
				}
				if len(c.Catalog.Packages) != 1 || len(c.Desired) != 1 {
					t.Fatal("carrier must contain only its agent")
				}
				if c.Lock.Platforms["amd64"][name].Version != c.Lock.Platforms["arm64"][name].Version {
					t.Fatal("architecture versions differ")
				}
				if len(c.Catalog.Packages[name].Notices) == 0 {
					t.Fatal("agent notices missing")
				}
				source := t.TempDir()
				if err := Build(context.Background(), server.Client(), map[string]Spec{name: specs[name]}, c.Catalog.Packages, os.DirFS(directory), arch, source, io.Discard); err != nil {
					t.Fatal(err)
				}
				want := string(payloads[name])
				if name == "agy" {
					want = "antigravity cli"
				}
				b, err := os.ReadFile(filepath.Join(source, "bin", name))
				if err != nil || string(b) != want {
					t.Fatalf("agent payload changed: %v", err)
				}
				for _, notice := range c.Catalog.Packages[name].Notices {
					b, err := readInstalledFile(t, source, "licenses/"+name+"/"+filepath.Base(notice))
					if err != nil {
						t.Fatal(err)
					}
					expected, err := os.ReadFile(filepath.Join(directory, notice))
					if err != nil || string(b) != string(expected) {
						t.Fatalf("notice changed: %s %v", notice, err)
					}
				}
			})
		}
	}
}
