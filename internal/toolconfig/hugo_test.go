package toolconfig

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestHugoStandardCatalogPins(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		t.Run(fmt.Sprintf("upgrade=%t", upgrade), func(t *testing.T) {
			c, err := Load("../../bundles/toolbox")
			if err != nil {
				t.Fatal(err)
			}
			if upgrade {
				for arch, checksum := range map[string]string{
					"amd64": "fada83ad56bb27ed4cb7dd81590303bdac16140f944ac4e66fc248117e2d5594",
					"arm64": "91ce984e998a955a3dc1b5852eb4e65d0ed0f6d12a5c51690df97a548e1fdeeb",
				} {
					pin := c.Lock.Platforms[arch]["hugo"]
					pin.Version = "v0.168.0"
					pin.URL = fmt.Sprintf("https://github.com/gohugoio/hugo/releases/download/v0.168.0/hugo_0.168.0_linux-%s.tar.gz", arch)
					pin.SHA256 = checksum
					c.Lock.Platforms[arch]["hugo"] = pin
				}
			}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			assertHugoStandardCatalogPins(t, c)
		})
	}
}

func assertHugoStandardCatalogPins(t *testing.T, c *Config) {
	t.Helper()
	p, ok := c.Catalog.Packages["hugo"]
	if !ok || p.Repository != "gohugoio/hugo" || p.Asset != "hugo_{version}_linux-{arch}.tar.gz" || p.Install.Kind != "tar.gz" || p.Install.Members["hugo"] != "bin/hugo" {
		t.Fatal("missing official standard Hugo recipe")
	}
	if len(p.Install.ArchiveNotices) != 1 || p.Install.ArchiveNotices[0] != "LICENSE" || len(p.Notices) != 1 || p.Notices[0] != "licenses/hugo/SOURCE" {
		t.Fatal("missing Hugo license provenance")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["hugo"]
		version, err := p.Version(pin.Version)
		asset := fmt.Sprintf("hugo_%s_linux-%s.tar.gz", version, arch)
		if err != nil || !c.Desired["hugo"].Contains(version) || pin.URL != "https://github.com/gohugoio/hugo/releases/download/"+pin.Version+"/"+asset || pin.Verification != "publisher-sha256" || len(pin.SHA256) != 64 {
			t.Fatalf("missing verified standard Hugo pin within desired range: %s: %+v", arch, pin)
		}
	}
}

// The documentation wrapper has its own pinned release and checksum manifest;
// toolbox upgrades must not require changing this separate dependency.
func TestHugoDocsFallbackPins(t *testing.T) {
	wrapper, err := os.ReadFile("../../scripts/hugo.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "\nversion=0.167.0\n") {
		t.Fatal("unexpected documentation Hugo version")
	}
	checksums, err := os.ReadFile("../../.config/hugo.sha256")
	if err != nil {
		t.Fatal(err)
	}
	for arch, checksum := range map[string]string{
		"amd64": "4d84519b9f619e6d4c3fb45a50157abeabeb724f859c60605f44c23def6e1169",
		"arm64": "d0dd0d1ed249842525a93413e10ca1ef6f3b63311ee1695b16a0ce488142cec1",
	} {
		want := checksum + "  hugo_0.167.0_linux-" + arch + ".tar.gz\n"
		if !strings.Contains(string(checksums), want) {
			t.Fatalf("missing verified documentation Hugo checksum: %s", arch)
		}
	}
}
