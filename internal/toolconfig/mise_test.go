package toolconfig

import (
	"strings"
	"testing"
)

func TestMiseMinimumAndOfficialMuslPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["mise"]
	if c.Desired["mise"].String() != ">=2026.10.8" {
		t.Fatal("mise minimum is not retained")
	}
	for _, tc := range []struct {
		tag     string
		allowed bool
	}{
		{"v2026.10.7", false}, {"v2026.10.8", true}, {"v2026.11.0", true}, {"v2026.10.9-rc1", false},
	} {
		version, err := p.Version(tc.tag)
		if err != nil {
			if tc.allowed {
				t.Fatal(err)
			}
			continue
		}
		if c.Desired["mise"].Contains(version) != tc.allowed {
			t.Fatalf("unexpected acceptance of %s", tc.tag)
		}
	}
	for arch, upstream := range map[string]string{"amd64": "x64", "arm64": "arm64"} {
		pin := c.Lock.Platforms[arch]["mise"]
		want := "https://github.com/jdx/mise/releases/download/" + pin.Version + "/mise-" + pin.Version + "-linux-" + upstream + "-musl"
		if pin.URL != want || p.Architectures[arch] != upstream || !strings.HasSuffix(p.Asset, "-musl") || pin.Verification != "publisher-sha256" {
			t.Fatalf("wrong official MUSL %s pin: %+v", arch, pin)
		}
	}
}
