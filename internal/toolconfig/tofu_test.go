package toolconfig

import (
	"strings"
	"testing"
)

func TestToolboxOpenTofuOfficialStableRecipe(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["opentofu"]
	if !ok {
		t.Fatal("opentofu missing from toolbox catalog")
	}
	if p.Provider != "github" || p.Repository != "opentofu/opentofu" || p.Normalization != "semver" || p.TagPrefix != "v" || p.Asset != "tofu_{version}_linux_{arch}.tar.gz" || p.Verification != "official-or-download" {
		t.Fatalf("unexpected official recipe: %+v", p)
	}
	if p.Architectures["amd64"] != "amd64" || p.Architectures["arm64"] != "arm64" || len(p.Architectures) != 2 {
		t.Fatal("wrong upstream architecture names")
	}
	if p.Sidecar != "tofu_{version}_SHA256SUMS" {
		t.Fatal("missing publisher checksum sidecar")
	}
	if p.Install.Kind != "tar.gz" || len(p.Install.Members) != 1 || p.Install.Members["tofu"] != "bin/tofu" || p.Install.Preserve {
		t.Fatalf("unexpected installation: %+v", p.Install)
	}
	if got := strings.Join(p.Install.ArchiveNotices, ","); got != "LICENSE,README.md,CHANGELOG.md" {
		t.Fatalf("MPL-2.0 license must be retained from the release archive: %q", got)
	}
	if len(p.Notices) != 1 || p.Notices[0] != "licenses/opentofu/SOURCE" {
		t.Fatalf("missing source record: %v", p.Notices)
	}
	if _, ok := c.Desired["opentofu"]; !ok {
		t.Fatal("missing stable requirement")
	}
	assertRollingPublisherPins(t, c, "opentofu")
	// The toolbox must track the same stable pin as the separate bundle.
	o, err := Load("../../bundles/opentofu")
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if c.Lock.Platforms[arch]["opentofu"] != o.Lock.Platforms[arch]["opentofu"] {
			t.Fatalf("%s pin differs from the separate opentofu bundle", arch)
		}
	}
}
