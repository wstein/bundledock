package toolconfig

import (
	"strings"
	"testing"
)

func TestStwOfficialStableRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["stw"]
	if !ok {
		t.Fatal("stw missing from catalog")
	}
	if p.Provider != "github" || p.Repository != "wstein/sourcesteward" || p.Normalization != "semver" || p.TagPrefix != "v" || p.Asset != "stw_{version}_linux_{arch}.tar.gz" || p.Verification != "official-or-download" {
		t.Fatalf("unexpected official recipe: %+v", p)
	}
	if p.Architectures["amd64"] != "amd64" || p.Architectures["arm64"] != "arm64" || len(p.Architectures) != 2 {
		t.Fatal("wrong upstream architecture names")
	}
	if p.Sidecar != "SHA256SUMS" {
		t.Fatal("missing publisher checksum sidecar")
	}
	if p.Install.Kind != "tar.gz" || len(p.Install.Members) != 1 || p.Install.Members["stw"] != "bin/stw" || p.Install.Preserve {
		t.Fatalf("unexpected installation: %+v", p.Install)
	}
	if got := strings.Join(p.Install.ArchiveNotices, ","); got != "LICENSE,README.md" {
		t.Fatalf("license must be retained from the release archive: %q", got)
	}
	if got := strings.Join(p.Install.ArchiveNoticeTrees, ","); got != "third_party" {
		t.Fatalf("third-party notices must be retained: %q", got)
	}
	if len(p.Notices) != 1 || p.Notices[0] != "licenses/stw/SOURCE" {
		t.Fatalf("missing source record: %v", p.Notices)
	}
	if _, ok := c.Desired["stw"]; !ok {
		t.Fatal("missing stable requirement")
	}
	version := ""
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["stw"]
		normalized, err := p.Version(pin.Version)
		if err != nil || strings.Contains(normalized, "-") || !c.Desired["stw"].Contains(normalized) {
			t.Fatalf("not an allowed stable pin: %+v %v", pin, err)
		}
		if version != "" && version != pin.Version {
			t.Fatal("architecture versions differ")
		}
		version = pin.Version
		want := "https://github.com/wstein/sourcesteward/releases/download/" + pin.Version + "/stw_" + normalized + "_linux_" + p.Architectures[arch] + ".tar.gz"
		if pin.URL != want || len(pin.SHA256) != 64 || pin.Verification != "publisher-sha256" {
			t.Fatalf("invalid publisher-verified %s pin: %+v", arch, pin)
		}
	}
}
