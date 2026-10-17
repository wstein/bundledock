package toolconfig

import (
	"regexp"
	"strings"
	"testing"
)

// gh resolves extensions only from <gh data dir>/extensions/<name>/gh-<name>.
// The official raw binary is staged at a fixed read-only toolbox path; the
// consumer links it into that directory (see docs/toolbox.md).
func TestGHStackExtensionRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["gh-stack"]
	if !ok {
		t.Fatal("gh-stack missing from catalog")
	}
	const dest = "libexec/gh-stack"
	if p.Repository != "github/gh-stack" || p.TagPrefix != "v" || p.Asset != "linux-{arch}" || p.Install.Kind != "raw" || len(p.Install.Members) != 1 || p.Install.Members["gh-stack"] != dest {
		t.Fatalf("unexpected recipe: %+v", p)
	}
	if len(p.Notices) == 0 {
		t.Fatal("missing retained notices")
	}
	sha := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, arch := range []string{"amd64", "arm64"} {
		s := c.Lock.Platforms[arch]["gh-stack"]
		want := "https://github.com/github/gh-stack/releases/download/" + s.Version + "/linux-" + arch
		if !strings.HasPrefix(s.Version, "v") || s.URL != want || s.Verification != "publisher-sha256" || !sha.MatchString(s.SHA256) {
			t.Fatalf("%s lock pin is wrong: %+v", arch, s)
		}
	}
}
