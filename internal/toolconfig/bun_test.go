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
	for arch, sum := range map[string]string{"amd64": "1fc2edac843102909e3a1be1d8d9802cc6071cf074e67e88231f4ffe0f8b397b", "arm64": "efa9813da5ed72423bf847f916e8d2c47c0d776add972354026a75e10da9aa21"} {
		if c.Lock.Platforms[arch]["bun"].SHA256 != sum {
			t.Fatal("wrong official checksum")
		}
	}
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
