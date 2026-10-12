package toolconfig

import "testing"

func TestOptionalOpenTofuOfficialPins(t *testing.T) {
	c, err := Load("../../bundles/opentofu")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog.Packages) != 1 || len(c.Desired) != 1 {
		t.Fatal("OpenTofu must pin only its CLI")
	}
	p := c.Catalog.Packages["opentofu"]
	if p.Repository != "opentofu/opentofu" || p.Install.Members["tofu"] != "bin/tofu" {
		t.Fatal("official CLI recipe required")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		s := c.Lock.Platforms[arch]["opentofu"]
		if s.Version != "v1.13.1" || s.Verification != "publisher-sha256" {
			t.Fatal("stable publisher checksum pins required")
		}
	}
}

func TestOpenTofuRejectsUnofficialRecipeAndPins(t *testing.T) {
	for _, kind := range []string{"repository", "architecture", "members", "asset", "sidecar", "version", "url", "verification"} {
		t.Run(kind, func(t *testing.T) {
			c, err := Load("../../bundles/opentofu")
			if err != nil {
				t.Fatal(err)
			}
			p := c.Catalog.Packages["opentofu"]
			s := c.Lock.Platforms["amd64"]["opentofu"]
			switch kind {
			case "repository":
				p.Repository = "other/opentofu"
			case "architecture":
				p.Architectures["amd64"] = "x64"
			case "members":
				p.Install.Members["tofu"] = "bin/terraform"
			case "asset":
				p.Asset = "other_{version}_{arch}.tar.gz"
			case "sidecar":
				p.Sidecar = "SHA256SUMS"
			case "version":
				s.Version = "1.13.1"
			case "url":
				s.URL = "https://github.com/other/opentofu/releases/download/v1.13.1/tofu_1.13.1_linux_amd64.tar.gz"
			case "verification":
				s.Verification = "tls-download-sha256"
			}
			c.Catalog.Packages["opentofu"] = p
			c.Lock.Platforms["amd64"]["opentofu"] = s
			c.Bind()
			if c.Validate() == nil {
				t.Fatal("accepted unofficial OpenTofu " + kind)
			}
		})
	}
}
