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
	p, ok := c.Catalog.Packages["hugo"]
	if !ok || p.Repository != "gohugoio/hugo" || p.Asset != "hugo_{version}_linux-{arch}.tar.gz" || p.Install.Kind != "tar.gz" || p.Install.Members["hugo"] != "bin/hugo" {
		t.Fatal("missing official standard Hugo recipe")
	}
	if len(p.Install.ArchiveNotices) != 1 || p.Install.ArchiveNotices[0] != "LICENSE" || len(p.Notices) != 1 || p.Notices[0] != "licenses/hugo/SOURCE" {
		t.Fatal("missing Hugo license provenance")
	}
	checksums, err := os.ReadFile("../../.config/hugo.sha256")
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch]["hugo"]
		asset := fmt.Sprintf("hugo_0.167.0_linux-%s.tar.gz", arch)
		if pin.Version != "v0.167.0" || pin.URL != "https://github.com/gohugoio/hugo/releases/download/v0.167.0/"+asset || pin.Verification != "publisher-sha256" || len(pin.SHA256) != 64 || !strings.Contains(string(checksums), pin.SHA256+"  "+asset) {
			t.Fatalf("missing verified standard Hugo pin consistent with docs fallback: %s", arch)
		}
	}
}
