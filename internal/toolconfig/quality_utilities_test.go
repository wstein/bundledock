package toolconfig

import "testing"

func TestQualityUtilitiesOfficialRecipes(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for name, repo := range map[string]string{"deadbranch": "armgabrielyan/deadbranch", "shellcheck": "koalaman/shellcheck", "shfmt": "mvdan/sh", "just": "casey/just", "ruff": "astral-sh/ruff", "delta": "dandavison/delta", "watchexec": "watchexec/watchexec", "hyperfine": "sharkdp/hyperfine", "age": "FiloSottile/age"} {
		t.Run(name, func(t *testing.T) {
			p, ok := c.Catalog.Packages[name]
			if !ok {
				t.Fatal("requested utility missing")
			}
			if p.Repository != repo || p.Install.Members[name] != "bin/"+name || len(p.Notices) == 0 {
				t.Fatal("incomplete official executable recipe or notices")
			}
			for _, arch := range []string{"amd64", "arm64"} {
				s := c.Lock.Platforms[arch][name]
				if s.Version == "" || len(s.SHA256) != 64 {
					t.Fatalf("missing %s pin", arch)
				}
			}
		})
	}
}
