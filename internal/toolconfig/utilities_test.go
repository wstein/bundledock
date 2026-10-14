package toolconfig

import "testing"

func TestToolboxUtilities(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, repo, kind string }{
		{"fzf", "junegunn/fzf", "tar.gz"},
		{"bat", "sharkdp/bat", "tar.gz"},
		{"regctl", "regclient/regclient", "raw"},
		{"mise", "jdx/mise", "raw"},
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
				// Updaters may advance pins within the declared package constraints.
				version, err := p.Version(s.Version)
				if err != nil || !c.Desired[tc.name].Contains(version) || len(s.SHA256) != 64 {
					t.Fatalf("missing stable %s pin within desired range: %+v", arch, s)
				}
			}
		})
	}
}
