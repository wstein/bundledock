package toolconfig

import (
	"strings"
	"testing"
)

func TestJujutsuOfficialMuslPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["jj"]
	if !ok || p.Repository != "jj-vcs/jj" || p.Asset != "jj-v{version}-{arch}-unknown-linux-musl.tar.gz" || p.Install.Kind != "tar.gz" || p.Install.Members["jj"] != "bin/jj" {
		t.Fatal("missing official Jujutsu prebuilt recipe")
	}
	if len(p.Install.ArchiveNotices) != 1 || p.Install.ArchiveNotices[0] != "LICENSE" || len(p.Notices) != 1 || p.Notices[0] != "licenses/jj/SOURCE" {
		t.Fatal("missing full upstream Jujutsu license and provenance")
	}
	assertRollingPublisherPins(t, c, "jj")
	for arch, target := range map[string]string{"amd64": "x86_64", "arm64": "aarch64"} {
		pin := c.Lock.Platforms[arch]["jj"]
		if p.Architectures[arch] != target || !strings.HasSuffix(pin.URL, "/"+pin.Version+"/jj-"+pin.Version+"-"+target+"-unknown-linux-musl.tar.gz") || pin.Verification != "publisher-sha256" || len(pin.SHA256) != 64 {
			t.Fatalf("bad %s official pin: %+v", arch, pin)
		}
	}
}
