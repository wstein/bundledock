package toolconfig

import (
	"os"
	"strings"
	"testing"
)

func TestLeafOfficialRawPinsAndVersionBoundNotices(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["leaf"]
	if !ok || p.Repository != "RivoLink/leaf" || p.Asset != "leaf-linux-{arch}" || p.Install.Kind != "raw" || p.Install.Members["leaf"] != "bin/leaf" {
		t.Fatal("missing official Leaf raw binary recipe")
	}
	source, err := os.ReadFile("../../bundles/toolbox/licenses/leaf/SOURCE")
	if err != nil {
		t.Fatal(err)
	}
	for arch, target := range map[string]string{"amd64": "x86_64", "arm64": "arm64"} {
		pin := c.Lock.Platforms[arch]["leaf"]
		if p.Architectures[arch] != target || pin.Version != "1.29.0" || !strings.HasSuffix(pin.URL, "/1.29.0/leaf-linux-"+target) || pin.Verification != "publisher-sha256" || len(pin.SHA256) != 64 {
			t.Fatalf("bad %s pin: %+v", arch, pin)
		}
		if !strings.Contains(string(source), "Pinned release: "+pin.Version+"\n") {
			t.Fatal("notice version differs from lock")
		}
	}
	if len(p.Notices) != 2 || p.Notices[0] != "licenses/leaf/LICENSE" || p.Notices[1] != "licenses/leaf/SOURCE" {
		t.Fatal("missing full MIT license and provenance")
	}
}
