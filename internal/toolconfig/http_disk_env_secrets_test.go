package toolconfig

import (
	"strings"
	"testing"
)

func TestHTTPDiskEnvSecretsOfficialRecipes(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, repo, kind string }{
		{"xh", "ducaale/xh", "tar.gz"}, {"dust", "bootandy/dust", "tar.gz"},
		{"direnv", "direnv/direnv", "raw"}, {"sops", "getsops/sops", "raw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := c.Catalog.Packages[tc.name]
			if !ok {
				t.Fatal("requested tool missing")
			}
			if p.Repository != tc.repo || p.Install.Kind != tc.kind || p.Install.Members[tc.name] != "bin/"+tc.name || p.Normalization != "semver" || p.TagPrefix != "v" {
				t.Fatal("incomplete official prebuilt recipe")
			}
			assertRollingPublisherPins(t, c, tc.name)
			for _, arch := range []string{"amd64", "arm64"} {
				pin := c.Lock.Platforms[arch][tc.name]
				version, _ := p.Version(pin.Version)
				asset := strings.NewReplacer("{version}", version, "{arch}", p.Architectures[arch]).Replace(p.Asset)
				want := "https://github.com/" + tc.repo + "/releases/download/" + pin.Version + "/" + asset
				if pin.URL != want {
					t.Fatalf("wrong official %s artifact: %s", arch, pin.URL)
				}
			}
		})
	}
}
