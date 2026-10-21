package toolconfig

import (
	"strings"
	"testing"
)

func TestZkOfficialStableRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["zk"]
	if !ok {
		t.Fatal("zk missing from catalog")
	}
	if p.Provider != "github" || p.Repository != "zk-org/zk" || p.Normalization != "semver" || p.TagPrefix != "v" || p.Asset != "zk-v{version}-linux-{arch}.tar.gz" || p.Verification != "official-or-download" {
		t.Fatalf("unexpected official recipe: %+v", p)
	}
	if p.Architectures["amd64"] != "amd64" || p.Architectures["arm64"] != "arm64" || len(p.Architectures) != 2 {
		t.Fatal("wrong upstream architecture names")
	}
	if p.Sidecar != "" {
		t.Fatal("zk publishes no checksum sidecar; the release API digest is the publisher checksum")
	}
	if p.Install.Kind != "tar.gz" || len(p.Install.Members) != 1 || p.Install.Members["zk"] != "bin/zk" || p.Install.Preserve {
		t.Fatalf("unexpected installation: %+v", p.Install)
	}
	if len(p.Install.ArchiveNotices) != 0 {
		t.Fatalf("the zk archive carries only the executable: %v", p.Install.ArchiveNotices)
	}
	if got := strings.Join(p.Notices, ","); got != "licenses/zk/LICENSE,licenses/zk/SOURCE" {
		t.Fatalf("missing GPL-3.0 text or source record: %v", p.Notices)
	}
	if _, ok := c.Desired["zk"]; !ok {
		t.Fatal("missing stable requirement")
	}
	version := ""
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["zk"]
		normalized, err := p.Version(pin.Version)
		if err != nil || strings.Contains(normalized, "-") || !c.Desired["zk"].Contains(normalized) {
			t.Fatalf("not an allowed stable pin: %+v %v", pin, err)
		}
		if version != "" && version != pin.Version {
			t.Fatal("architecture versions differ")
		}
		version = pin.Version
		want := "https://github.com/zk-org/zk/releases/download/" + pin.Version + "/zk-v" + normalized + "-linux-" + p.Architectures[arch] + ".tar.gz"
		if pin.URL != want || len(pin.SHA256) != 64 || pin.Verification != "publisher-sha256" {
			t.Fatalf("invalid publisher-verified %s pin: %+v", arch, pin)
		}
	}
}
