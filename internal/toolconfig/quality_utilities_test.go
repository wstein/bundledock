package toolconfig

import "testing"

func TestQualityUtilitiesOfficialRecipes(t *testing.T) {
	for name, repo := range map[string]string{"shellcheck": "koalaman/shellcheck", "shfmt": "mvdan/sh", "just": "casey/just", "ruff": "astral-sh/ruff", "delta": "dandavison/delta", "watchexec": "watchexec/watchexec", "hyperfine": "sharkdp/hyperfine", "age": "FiloSottile/age", "lazygit": "jesseduffield/lazygit", "git-lfs": "git-lfs/git-lfs"} {
		t.Run(name, func(t *testing.T) {
			c := recipeConfig(t, name)
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
