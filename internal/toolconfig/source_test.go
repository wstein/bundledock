package toolconfig

import (
	"strings"
	"testing"
)

// The hadolint example of docs/design/third-party-source-mirror/data-model.md.
const hadolintSource = `{
  "obligation": "source-offer",
  "licenses": ["GPL-2.0-only", "GPL-3.0-only", "GPL-3.0-or-later", "MPL-2.0"],
  "basis": "declared",
  "upstream": {
    "repository": "https://github.com/hadolint/hadolint",
    "tarball": "https://api.github.com/repos/hadolint/hadolint/tarball/{lock_version}"
  },
  "recipe": {
    "kind": "cabal-freeze",
    "freeze_source": "ThirdPartyNotices.txt",
    "excluded": ["GHC 9.10.2 and boot libraries", "alpine:3.24 toolchain"],
    "static_system_libs": ["gmp", "ncurses", "zlib", "musl", "libffi"]
  }
}`

func decodeSource(t *testing.T, raw string) (*Source, error) {
	t.Helper()
	s := new(Source)
	return s, Decode(strings.NewReader(raw), s)
}

// withSource loads the real toolbox, attaches the block to the named package and
// rebinds the digests, so only the source block can make Validate fail.
func withSource(t *testing.T, name string, s *Source) error {
	t.Helper()
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages[name]
	p.Source = s
	c.Catalog.Packages[name] = p
	c.Bind()
	return c.Validate()
}

func TestSourceBlockAcceptsDataModelExample(t *testing.T) {
	s, err := decodeSource(t, hadolintSource)
	if err != nil {
		t.Fatal(err)
	}
	if s.Obligation != "source-offer" || s.Basis != "declared" || s.Upstream.Repository != "https://github.com/hadolint/hadolint" || s.Recipe.Kind != "cabal-freeze" || s.Recipe.FreezeSource != "ThirdPartyNotices.txt" || len(s.Recipe.StaticSystemLibs) != 5 || len(s.Recipe.Excluded) != 2 || len(s.Licenses) != 4 {
		t.Fatalf("source block misparsed: %+v", s)
	}
	if err := withSource(t, "hadolint", s); err != nil {
		t.Fatal(err)
	}
}

func TestSourceBlockRejectsUnknownKeys(t *testing.T) {
	for _, raw := range []string{
		`{"obligation":"none","extra":1}`,
		`{"obligation":"source-offer","upstream":{"repository":"https://github.com/a/b","mirror":"x"}}`,
		`{"obligation":"source-offer","recipe":{"kind":"cargo-vendor","argv":["cargo"]}}`,
	} {
		if _, err := decodeSource(t, raw); err == nil {
			t.Errorf("accepted unknown key in %s", raw)
		}
	}
}

func TestCatalogWithoutSourceBlocksKeepsItsDigest(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	before := c.CatalogDigest()
	if before != c.Lock.CatalogDigest {
		t.Fatal("lock digest differs from the catalog")
	}
	p := c.Catalog.Packages["shellcheck"]
	p.Source = &Source{Obligation: "undetermined"}
	c.Catalog.Packages["shellcheck"] = p
	if c.CatalogDigest() == before {
		t.Fatal("source block is not covered by catalog_digest")
	}
}
