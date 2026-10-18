package toolconfig

import (
	"strings"
	"testing"
)

func TestFastfetchOfficialStableRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["fastfetch"]
	if !ok {
		t.Fatal("fastfetch missing from catalog")
	}
	if p.Provider != "github" || p.Repository != "fastfetch-cli/fastfetch" || p.Normalization != "semver" || p.TagPrefix != "" || p.Asset != "fastfetch-linux-{arch}.tar.gz" || p.Verification != "official-or-download" {
		t.Fatalf("unexpected official recipe: %+v", p)
	}
	if p.Architectures["amd64"] != "amd64" || p.Architectures["arm64"] != "aarch64" {
		t.Fatal("wrong upstream architecture names")
	}
	if p.Install.Kind != "tar.gz" || len(p.Install.Members) != 1 || p.Install.Members["fastfetch"] != "bin/fastfetch" || p.Install.Preserve {
		t.Fatalf("unexpected installation: %+v", p.Install)
	}
	if _, ok := c.Desired["fastfetch"]; !ok {
		t.Fatal("missing stable requirement")
	}
	version := ""
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["fastfetch"]
		normalized, err := p.Version(pin.Version)
		if err != nil || strings.Contains(normalized, "-") || !c.Desired["fastfetch"].Contains(normalized) {
			t.Fatalf("not an allowed stable pin: %+v %v", pin, err)
		}
		if version != "" && version != pin.Version {
			t.Fatal("architecture versions differ")
		}
		version = pin.Version
		want := "https://github.com/fastfetch-cli/fastfetch/releases/download/" + pin.Version + "/fastfetch-linux-" + p.Architectures[arch] + ".tar.gz"
		if pin.URL != want || len(pin.SHA256) != 64 || pin.Verification != "publisher-sha256" {
			t.Fatalf("invalid publisher-verified %s pin: %+v", arch, pin)
		}
	}
}
