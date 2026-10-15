package toolconfig

import "testing"

func TestOptionalBunOfficialPinsAndRecipe(t *testing.T) {
	c, err := Load("../../bundles/bun")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog.Packages) != 1 || c.Catalog.Packages["bun"].Install.Profile != "bun-runtime" {
		t.Fatal("separate Bun profile required")
	}
	assertRollingPublisherPins(t, c, "bun")
	for _, kind := range []string{"provider", "repository", "architecture", "profile", "root", "version", "url", "checksum provenance"} {
		t.Run(kind, func(t *testing.T) {
			c, err := Load("../../bundles/bun")
			if err != nil {
				t.Fatal(err)
			}
			p := c.Catalog.Packages["bun"]
			s := c.Lock.Platforms["amd64"]["bun"]
			switch kind {
			case "provider":
				p.Provider = "claude"
			case "repository":
				p.Repository = "other/bun"
			case "architecture":
				p.Architectures["amd64"] = "x64"
			case "profile":
				p.Install.Profile = ""
			case "root":
				p.Install.Members["bun"] = "bin/bun"
			case "version":
				s.Version = "v1.4.3"
			case "url":
				s.URL = "https://github.com/oven-sh/bun/releases/download/bun-v1.4.3/bun-linux-x64.zip"
			case "checksum provenance":
				s.Verification = "tls-download-sha256"
			}
			c.Catalog.Packages["bun"] = p
			c.Lock.Platforms["amd64"]["bun"] = s
			c.Bind()
			if c.Validate() == nil {
				t.Fatal("accepted invalid Bun recipe/pin")
			}
		})
	}
}
