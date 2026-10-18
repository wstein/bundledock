package toolconfig

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These digests identify the reviewed historical v4.1.1 source notices,
// independently of any later allowed binary pin.
func TestSCCHistoricalLicenseSnapshot(t *testing.T) {
	const base = "../../bundles/toolbox/licenses/scc"
	license, err := os.ReadFile(filepath.Join(base, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(license)) != "394715fd981d15253bdc4677286abfce01da356040381a0d9de7701bd7c8069c" {
		t.Fatal("historical MIT license bytes changed")
	}
	source, err := os.ReadFile(filepath.Join(base, "SOURCE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Upstream: https://github.com/wstein/scc-fork\n", "Historical license snapshot: v4.1.1\n", "Source commit: 9bb762717271e08953a01cfe63981af21390d777\n"} {
		if !strings.Contains(string(source), field) {
			t.Fatalf("missing provenance %q", field)
		}
	}
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, name := range c.Catalog.Packages["scc"].Notices {
		listed[name] = true
	}
	h := sha256.New()
	err = fs.WalkDir(os.DirFS(filepath.Join(base, "dependencies")), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !listed["licenses/scc/dependencies/"+name] {
			t.Errorf("dependency notice omitted from bundle: %s", name)
		}
		data, err := os.ReadFile(filepath.Join(base, "dependencies", name))
		if err != nil {
			return err
		}
		h.Write([]byte(name + "\x00"))
		h.Write(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != "62ff92df3380438b907c124ae3df92de9f3f56deda37bffb17e8e23c9df9b587" {
		t.Fatal("historical dependency notice set or bytes changed")
	}
}
