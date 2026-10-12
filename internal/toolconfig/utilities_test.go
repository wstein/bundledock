package toolconfig

import "testing"

func TestToolboxUtilities(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, repo, kind, version string }{
		{"fzf", "junegunn/fzf", "tar.gz", "v0.74.5"},
		{"bat", "sharkdp/bat", "tar.gz", "v0.26.1"},
		{"regctl", "regclient/regclient", "raw", "v0.11.6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := c.Catalog.Packages[tc.name]
			if !ok {
				t.Fatal("requested toolbox utility missing")
			}
			if p.Repository != tc.repo || p.Install.Kind != tc.kind || p.Install.Members[tc.name] != "bin/"+tc.name {
				t.Fatal("missing official executable recipe")
			}
			if len(p.Notices) == 0 {
				t.Fatal("missing retained notices")
			}
			for _, arch := range []string{"amd64", "arm64"} {
				s := c.Lock.Platforms[arch][tc.name]
				if s.Version != tc.version || len(s.SHA256) != 64 {
					t.Fatalf("missing stable %s pin", arch)
				}
			}
		})
	}
}
