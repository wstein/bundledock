package toolconfig

import "testing"

// Developer tools added after the first utilities set. Each is an official
// prebuilt release; bin differs from the catalog ID for difftastic.
func TestDeveloperToolRecipes(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, repo, kind, bin, prefix string }{
		{"actionlint", "rhysd/actionlint", "tar.gz", "actionlint", "v"},
		{"hadolint", "hadolint/hadolint", "raw", "hadolint", "v"},
		{"pinact", "suzuki-shunsuke/pinact", "tar.gz", "pinact", "v"},
		{"zizmor", "zizmorcore/zizmor", "tar.gz", "zizmor", "v"},
		{"cosign", "sigstore/cosign", "raw", "cosign", "v"},
		{"git-cliff", "orhun/git-cliff", "tar.gz", "git-cliff", "v"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := c.Catalog.Packages[tc.name]
			if !ok {
				t.Fatal("requested developer tool missing")
			}
			if p.Repository != tc.repo || p.TagPrefix != tc.prefix || p.Install.Kind != tc.kind || p.Install.Members[tc.bin] != "bin/"+tc.bin || len(p.Install.Members) != 1 {
				t.Fatal("missing official executable recipe")
			}
			if len(p.Notices) == 0 {
				t.Fatal("missing retained notices")
			}
			for _, arch := range []string{"amd64", "arm64"} {
				s := c.Lock.Platforms[arch][tc.name]
				if s.Version == "" || s.SHA256 == "" || s.Verification == "" {
					t.Fatalf("%s lock pin is incomplete", arch)
				}
			}
		})
	}
}
