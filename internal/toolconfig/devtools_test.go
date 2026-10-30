package toolconfig

import (
	"regexp"
	"strings"
	"testing"
)

// Developer tools added after the first utilities set. Each is an official
// prebuilt release; bin differs from the catalog ID for difftastic.
func TestDeveloperToolRecipes(t *testing.T) {
	for _, tc := range []struct{ name, repo, kind, bin, prefix string }{
		{"actionlint", "rhysd/actionlint", "tar.gz", "actionlint", "v"},
		{"hadolint", "hadolint/hadolint", "raw", "hadolint", "v"},
		{"pinact", "suzuki-shunsuke/pinact", "tar.gz", "pinact", "v"},
		{"zizmor", "zizmorcore/zizmor", "tar.gz", "zizmor", "v"},
		{"cosign", "sigstore/cosign", "raw", "cosign", "v"},
		{"git-cliff", "orhun/git-cliff", "tar.gz", "git-cliff", "v"},
		{"zoxide", "ajeetdsouza/zoxide", "tar.gz", "zoxide", "v"},
		{"difftastic", "Wilfred/difftastic", "tar.gz", "difft", ""},
		{"jsonschema-cli", "Stranger6667/jsonschema", "tar.gz", "jsonschema-cli", "cli-v"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := recipeConfig(t, tc.name)
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
			// The lock URL must be exactly the expanded catalog asset for each
			// architecture, so a wrong-architecture pin cannot pass.
			sha := regexp.MustCompile(`^[0-9a-f]{64}$`)
			for _, arch := range []string{"amd64", "arm64"} {
				s := c.Lock.Platforms[arch][tc.name]
				asset := strings.NewReplacer("{version}", strings.TrimPrefix(s.Version, tc.prefix), "{arch}", p.Architectures[arch]).Replace(p.Asset)
				want := "https://github.com/" + tc.repo + "/releases/download/" + s.Version + "/" + asset
				if s.URL != want {
					t.Fatalf("%s lock URL %q, want expanded catalog asset %q", arch, s.URL, want)
				}
				if s.Verification != "publisher-sha256" || !sha.MatchString(s.SHA256) {
					t.Fatalf("%s publisher-sha256 malformed: %+v", arch, s)
				}
				if !strings.HasPrefix(s.Version, tc.prefix) {
					t.Fatalf("%s version %q lacks tag prefix %q", arch, s.Version, tc.prefix)
				}
			}
			if c.Lock.Platforms["amd64"][tc.name].Version != c.Lock.Platforms["arm64"][tc.name].Version {
				t.Fatal("architectures pin different versions")
			}
			if c.Lock.Platforms["amd64"][tc.name].SHA256 == c.Lock.Platforms["arm64"][tc.name].SHA256 {
				t.Fatal("architectures share one digest")
			}
		})
	}
}

// Literal architecture tokens guard the catalog mapping itself.
func TestDeveloperToolArchitectureTokens(t *testing.T) {
	for name, want := range map[string][2]string{
		"hadolint":       {"x86_64", "arm64"},
		"zizmor":         {"x86_64", "aarch64"},
		"difftastic":     {"x86_64-unknown-linux-musl", "aarch64-unknown-linux-gnu"},
		"jsonschema-cli": {"x86_64", "aarch64"},
		"zoxide":         {"x86_64", "aarch64"},
		"git-cliff":      {"x86_64", "aarch64"},
	} {
		a := recipeConfig(t, name).Catalog.Packages[name].Architectures
		if a["amd64"] != want[0] || a["arm64"] != want[1] {
			t.Errorf("%s architecture tokens: %v", name, a)
		}
	}
}
