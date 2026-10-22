package toolconfig

import (
	"strings"
	"testing"
)

func TestWhrOfficialStableRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["whr"]
	if !ok {
		t.Fatal("whr missing from catalog")
	}
	if p.Provider != "github" || p.Repository != "wstein/workharbor" || p.Normalization != "semver" || p.TagPrefix != "v" || p.Asset != "whr_{version}_linux_{arch}.tar.gz" || p.Verification != "official-or-download" {
		t.Fatalf("unexpected official recipe: %+v", p)
	}
	if p.Architectures["amd64"] != "amd64" || p.Architectures["arm64"] != "arm64" || len(p.Architectures) != 2 {
		t.Fatal("wrong upstream architecture names")
	}
	if p.Sidecar != "SHA256SUMS" {
		t.Fatal("missing publisher checksum sidecar")
	}
	if p.Install.Kind != "tar.gz" || len(p.Install.Members) != 1 || p.Install.Members["whr"] != "bin/whr" || p.Install.Preserve {
		t.Fatalf("unexpected installation: %+v", p.Install)
	}
	if got := strings.Join(p.Install.ArchiveNotices, ","); got != "LICENSE,README.md" {
		t.Fatalf("license must be retained from the release archive: %q", got)
	}
	if got := strings.Join(p.Install.ArchiveNoticeTrees, ","); got != "third_party" {
		t.Fatalf("third-party notices must be retained: %q", got)
	}
	if len(p.Notices) != 1 || p.Notices[0] != "licenses/whr/SOURCE" {
		t.Fatalf("missing source record: %v", p.Notices)
	}
	if _, ok := c.Desired["whr"]; !ok {
		t.Fatal("missing stable requirement")
	}
	version := ""
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["whr"]
		normalized, err := p.Version(pin.Version)
		if err != nil || strings.Contains(normalized, "-") || !c.Desired["whr"].Contains(normalized) {
			t.Fatalf("not an allowed stable pin: %+v %v", pin, err)
		}
		if version != "" && version != pin.Version {
			t.Fatal("architecture versions differ")
		}
		version = pin.Version
		want := "https://github.com/wstein/workharbor/releases/download/" + pin.Version + "/whr_" + normalized + "_linux_" + p.Architectures[arch] + ".tar.gz"
		if pin.URL != want || len(pin.SHA256) != 64 || pin.Verification != "publisher-sha256" {
			t.Fatalf("invalid publisher-verified %s pin: %+v", arch, pin)
		}
	}
}
