package toolconfig

import (
	"strings"
	"testing"
)

func TestLeafOfficialRawPinsAndRetainedNotices(t *testing.T) {
	c, err := Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["leaf"]
	if !ok || p.Repository != "RivoLink/leaf" || p.Asset != "leaf-linux-{arch}" || p.Install.Kind != "raw" || p.Install.Members["leaf"] != "bin/leaf" {
		t.Fatal("missing official Leaf raw binary recipe")
	}
	assertRollingPublisherPins(t, c, "leaf")
	for arch, target := range map[string]string{"amd64": "x86_64", "arm64": "arm64"} {
		pin := c.Lock.Platforms[arch]["leaf"]
		if p.Architectures[arch] != target || !strings.HasSuffix(pin.URL, "/"+pin.Version+"/leaf-linux-"+target) || pin.Verification != "publisher-sha256" || len(pin.SHA256) != 64 {
			t.Fatalf("bad %s pin: %+v", arch, pin)
		}
	}
	if len(p.Notices) != 2 || p.Notices[0] != "licenses/leaf/LICENSE" || p.Notices[1] != "licenses/leaf/SOURCE" {
		t.Fatal("missing full MIT license and provenance")
	}
}
