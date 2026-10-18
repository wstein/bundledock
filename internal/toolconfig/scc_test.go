package toolconfig

import (
	"strings"
	"testing"
)

func TestSCCOfficialStableRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["scc"]
	if !ok {
		t.Fatal("scc missing from catalog")
	}
	if p.Provider != "github" || p.Repository != "wstein/scc-fork" || p.Normalization != "semver" || p.TagPrefix != "v" || p.Asset != "scc_Linux_{arch}.tar.gz" || p.Verification != "official-or-download" {
		t.Fatalf("unexpected official recipe: %+v", p)
	}
	if p.Architectures["amd64"] != "x86_64" || p.Architectures["arm64"] != "arm64" {
		t.Fatal("wrong upstream architecture names")
	}
	if p.Sidecar != "checksums.txt" {
		t.Fatal("missing publisher checksum sidecar")
	}
	if p.Install.Kind != "tar.gz" || len(p.Install.Members) != 1 || p.Install.Members["scc"] != "bin/scc" || p.Install.Preserve {
		t.Fatalf("unexpected installation: %+v", p.Install)
	}
	if _, ok := c.Desired["scc"]; !ok {
		t.Fatal("missing stable requirement")
	}
	version := ""
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["scc"]
		normalized, err := p.Version(pin.Version)
		if err != nil || strings.Contains(normalized, "-") || !c.Desired["scc"].Contains(normalized) {
			t.Fatalf("not an allowed stable pin: %+v %v", pin, err)
		}
		if version != "" && version != pin.Version {
			t.Fatal("architecture versions differ")
		}
		version = pin.Version
		want := "https://github.com/wstein/scc-fork/releases/download/" + pin.Version + "/scc_Linux_" + p.Architectures[arch] + ".tar.gz"
		if pin.URL != want || len(pin.SHA256) != 64 || pin.Verification != "publisher-sha256" {
			t.Fatalf("invalid publisher-verified %s pin: %+v", arch, pin)
		}
	}
}
