package toolconfig

import (
	"strings"
	"testing"
)

func TestMisspellOfficialStableRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["misspell"]
	if !ok {
		t.Fatal("misspell missing from catalog")
	}
	if p.Provider != "github" || p.Repository != "golangci/misspell" || p.Normalization != "semver" || p.TagPrefix != "v" || p.Asset != "misspell_{version}_linux_{arch}.tar.gz" || p.Verification != "official-or-download" {
		t.Fatalf("unexpected official recipe: %+v", p)
	}
	if p.Architectures["amd64"] != "amd64" || p.Architectures["arm64"] != "arm64" || len(p.Architectures) != 2 {
		t.Fatal("wrong upstream architecture names")
	}
	if p.Sidecar != "misspell_{version}_checksums.txt" {
		t.Fatalf("misspell publishes a checksums file: %q", p.Sidecar)
	}
	if p.Install.Kind != "tar.gz" || len(p.Install.Members) != 1 || p.Install.Members["misspell"] != "bin/misspell" || p.Install.Preserve {
		t.Fatalf("unexpected installation: %+v", p.Install)
	}
	if len(p.Install.ArchiveNotices) != 1 || p.Install.ArchiveNotices[0] != "LICENSE" {
		t.Fatalf("the archive carries the MIT LICENSE: %v", p.Install.ArchiveNotices)
	}
	if got := strings.Join(p.Notices, ","); got != "licenses/misspell/SOURCE" {
		t.Fatalf("missing source record: %v", p.Notices)
	}
	if got := c.Desired["misspell"].String(); got != ">=0.8.0" {
		t.Fatalf("unexpected requirement %q", got)
	}
	assertRollingPublisherPins(t, c, "misspell")
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["misspell"]
		normalized, err := p.Version(pin.Version)
		if err != nil {
			t.Fatal(err)
		}
		want := "https://github.com/golangci/misspell/releases/download/" + pin.Version + "/misspell_" + normalized + "_linux_" + p.Architectures[arch] + ".tar.gz"
		if pin.URL != want || len(pin.SHA256) != 64 {
			t.Fatalf("invalid %s pin: %+v", arch, pin)
		}
	}
}
