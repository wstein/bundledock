package toolconfig

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestHugoStandardCatalogPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	assertHugoStandardCatalogPins(t, c)
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
