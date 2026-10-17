package toolconfig

import (
	"fmt"
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

// validSource returns a fresh, valid cargo-vendor block for mutation.
func validSource() *Source {
	return &Source{
		Obligation: "file-level-source",
		Licenses:   []string{"MPL-2.0"},
		Basis:      "scanned",
		Upstream: &SourceUpstream{
			Repository: "https://github.com/koalaman/shellcheck",
			Tarball:    "https://github.com/koalaman/shellcheck/archive/refs/tags/{lock_version}.tar.gz",
		},
		Recipe: &SourceRecipe{Kind: "cargo-vendor"},
	}
}

func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%03d", prefix, i)
	}
	return out
}

func TestSourceBlockAcceptsLicenseExpressions(t *testing.T) {
	s := validSource()
	s.Licenses = []string{"(Apache-2.0 OR MIT) AND Unicode-3.0", "GPL-3.0-or-later WITH GCC-exception-3.1", "MPL-2.0"}
	if err := withSource(t, "shellcheck", s); err != nil {
		t.Fatal(err)
	}
}

// The version_regex rules apply to every class, not only to those that the
// strict version derivation would reject anyway.
func TestSourceBlockRejectsBadVersionRegexInEveryClass(t *testing.T) {
	for _, obligation := range []string{"none", "undetermined", "source-offer", "file-level-source"} {
		for name, regex := range map[string]string{"does not compile": "([0-9", "no capture group": `^v[0-9.]+$`, "two capture groups": `^(v)([0-9.]+)$`} {
			t.Run(obligation+" "+name, func(t *testing.T) {
				s := validSource()
				s.Obligation = obligation
				if obligation == "none" {
					s.Recipe = nil
				}
				s.VersionRegex = regex
				err := withSource(t, "shellcheck", s)
				if err == nil || !strings.Contains(err.Error(), "version_regex") {
					t.Fatalf("got %v", err)
				}
			})
		}
	}
}

func TestSourceBlockValidation(t *testing.T) {
	if err := withSource(t, "shellcheck", validSource()); err != nil {
		t.Fatalf("valid block refused: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Source)
	}{
		{"obligation not in enum", func(s *Source) { s.Obligation = "copyleft" }},
		{"obligation empty", func(s *Source) { s.Obligation = "" }},
		{"basis not in enum", func(s *Source) { s.Basis = "guessed" }},
		{"basis missing for a copyleft class", func(s *Source) { s.Basis = "" }},
		{"licenses missing for a copyleft class", func(s *Source) { s.Licenses = nil }},
		{"licenses unsorted", func(s *Source) { s.Licenses = []string{"MPL-2.0", "GPL-3.0-only"} }},
		{"licenses duplicated", func(s *Source) { s.Licenses = []string{"MPL-2.0", "MPL-2.0"} }},
		{"license with a space", func(s *Source) { s.Licenses = []string{"GPL 3"} }},
		{"upstream missing for a copyleft class", func(s *Source) { s.Upstream = nil }},
		{"recipe missing for a copyleft class", func(s *Source) { s.Recipe = nil }},
		{"none with a recipe", func(s *Source) { s.Obligation = "none" }},
		{"repository over http", func(s *Source) { s.Upstream.Repository = "http://github.com/koalaman/shellcheck" }},
		{"repository without owner", func(s *Source) { s.Upstream.Repository = "https://github.com/shellcheck" }},
		{"repository with dot dot", func(s *Source) { s.Upstream.Repository = "https://github.com/../shellcheck" }},
		{"tarball over http", func(s *Source) { s.Upstream.Tarball = "http://github.com/a/b/archive/refs/tags/{lock_version}.tar.gz" }},
		{"tarball with dot dot", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/../{lock_version}.tar.gz" }},
		{"tarball with control character", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}\n.tar.gz" }},
		{"tarball with userinfo", func(s *Source) { s.Upstream.Tarball = "https://user@github.com/a/b/archive/{lock_version}.tar.gz" }},
		{"tarball with a port", func(s *Source) { s.Upstream.Tarball = "https://github.com:8443/a/b/archive/{lock_version}.tar.gz" }},
		{"tarball with a query", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}.tar.gz?token=x" }},
		{"tarball with a fragment", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}.tar.gz#x" }},
		{"tarball with a unicode control character", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}\u0085.tar.gz" }},
		{"tarball with a backslash", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/\\{lock_version}.tar.gz" }},
		{"tarball with a space", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version} .tar.gz" }},
		{"tarball with a bidi format character", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}\u202e.tar.gz" }},
		{"tarball with a zero width space", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}\u200b.tar.gz" }},
		{"tarball without a placeholder", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/refs/tags/v1.2.3.tar.gz" }},
		{"tarball with a trailing empty query", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}.tar.gz?" }},
		{"tarball with a trailing empty fragment", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}.tar.gz#" }},
		{"tarball with an encoded newline", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%0a.tar.gz" }},
		{"tarball with an encoded NUL", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%00.tar.gz" }},
		{"tarball with an encoded DEL", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%7F.tar.gz" }},
		{"repository with a dot segment", func(s *Source) { s.Upstream.Repository = "https://github.com/./shellcheck" }},
		{"excluded with a bidi format character", func(s *Source) { s.Recipe.Excluded = []string{"gcc\u202e"} }},
		{"excluded duplicated", func(s *Source) { s.Recipe.Excluded = []string{"gcc", "gcc"} }},
		{"excluded too many", func(s *Source) { s.Recipe.Excluded = numbered("x", maxSourceEntries+1) }},
		{"excluded entry too long", func(s *Source) { s.Recipe.Excluded = []string{strings.Repeat("x", maxSourceText+1)} }},
		{"static libs too many", func(s *Source) { s.Recipe.StaticSystemLibs = numbered("lib", maxSourceEntries+1) }},
		{"licenses too many", func(s *Source) { s.Licenses = numbered("L", maxSourceEntries+1) }},
		{"license too long", func(s *Source) { s.Licenses = []string{strings.Repeat("A", maxSourceText+1)} }},
		{"version_regex too long", func(s *Source) { s.VersionRegex = "(" + strings.Repeat("a", maxSourceText) + ")" }},
		{"license expression ending in an operator", func(s *Source) { s.Licenses = []string{"MIT AND"} }},
		{"license expression starting with an operator", func(s *Source) { s.Licenses = []string{"AND MIT"} }},
		{"license expression with doubled operators", func(s *Source) { s.Licenses = []string{"MIT OR OR GPL-3.0-only"} }},
		{"license expression with a double space", func(s *Source) { s.Licenses = []string{"MIT  OR Apache-2.0"} }},
		{"license expression with an open parenthesis", func(s *Source) { s.Licenses = []string{"(MIT OR Apache-2.0"} }},
		{"license expression with a stray close parenthesis", func(s *Source) { s.Licenses = []string{"MIT)"} }},
		{"license expression with an identifier in operator position", func(s *Source) { s.Licenses = []string{"MIT Apache-2.0 GPL-3.0-only"} }},
		{"license expression starting with operators", func(s *Source) { s.Licenses = []string{"AND OR MIT"} }},
		{"license expression closing before opening", func(s *Source) { s.Licenses = []string{"MIT) OR (Apache-2.0"} }},
		{"license expression closing after an operator", func(s *Source) { s.Licenses = []string{"MIT OR) Apache-2.0"} }},
		{"license expression opening after an identifier", func(s *Source) { s.Licenses = []string{"MIT (OR Apache-2.0)"} }},
		{"license expression closing after an operator inside parentheses", func(s *Source) { s.Licenses = []string{"(MIT OR) Apache-2.0"} }},
		{"license expression with OR in identifier position", func(s *Source) { s.Licenses = []string{"OR AND MIT"} }},
		{"license expression with WITH in identifier position", func(s *Source) { s.Licenses = []string{"WITH AND MIT"} }},
		{"tarball too long", func(s *Source) {
			s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}/" + strings.Repeat("x", maxSourceText-25)
		}},
		{"tarball with a dot path segment", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/./{lock_version}.tar.gz" }},
		{"tarball with an encoded C1 control", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%C2%85.tar.gz" }},
		{"tarball with an encoded bidi override", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%E2%80%AE.tar.gz" }},
		{"tarball with an encoded invalid UTF-8 byte", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%FF.tar.gz" }},
		{"tarball with a double encoded newline", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%250a.tar.gz" }},
		{"tarball with a triple encoded newline", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}%25250a.tar.gz" }},
		{"license expression without an operator", func(s *Source) { s.Licenses = []string{"MIT Apache-2.0"} }},
		{"tarball on an unlisted host", func(s *Source) { s.Upstream.Tarball = "https://example.com/a/b/{lock_version}.tar.gz" }},
		{"tarball on an IP literal", func(s *Source) { s.Upstream.Tarball = "https://140.82.112.3/a/b/{lock_version}.tar.gz" }},
		{"tarball with an unknown placeholder", func(s *Source) { s.Upstream.Tarball = "https://github.com/a/b/archive/{lock_version}/{tag}.tar.gz" }},
		{"tarball empty", func(s *Source) { s.Upstream.Tarball = "" }},
		{"recipe kind unknown", func(s *Source) { s.Recipe.Kind = "make" }},
		{"recipe kind custom is reserved", func(s *Source) { s.Recipe.Kind = "custom" }},
		{"freeze_source on a non cabal recipe", func(s *Source) { s.Recipe.FreezeSource = "ThirdPartyNotices.txt" }},
		{"freeze_file on a non cabal recipe", func(s *Source) { s.Recipe.FreezeFile = "x.freeze" }},
		{"excluded entry empty", func(s *Source) { s.Recipe.Excluded = []string{""} }},
		{"static system lib with control character", func(s *Source) { s.Recipe.StaticSystemLibs = []string{"gmp\x00"} }},
		{"version_regex does not compile", func(s *Source) { s.VersionRegex = "([0-9" }},
		{"version_regex without a capture group", func(s *Source) { s.VersionRegex = `^v[0-9.]+$` }},
		{"version_regex with two capture groups", func(s *Source) { s.VersionRegex = `^(v)([0-9.]+)$` }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := validSource()
			tc.mutate(s)
			if err := withSource(t, "shellcheck", s); err == nil {
				t.Fatal("accepted an invalid source block")
			}
		})
	}
}

func TestSourceBlockCabalFreezeNeedsExactlyOneFreezeField(t *testing.T) {
	cabal := func(source, file string) *Source {
		s := validSource()
		s.Recipe = &SourceRecipe{Kind: "cabal-freeze", FreezeSource: source, FreezeFile: file}
		return s
	}
	for _, tc := range []struct {
		name         string
		source, file string
		ok           bool
	}{
		{"freeze_source only", "ThirdPartyNotices.txt", "", true},
		{"freeze_file only", "", "cabal.project.freeze", true},
		{"neither", "", "", false},
		{"both", "ThirdPartyNotices.txt", "cabal.project.freeze", false},
		{"freeze_source escaping", "../x", "", false},
		{"freeze_file with a directory", "", "dir/cabal.project.freeze", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := withSource(t, "shellcheck", cabal(tc.source, tc.file))
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v, err=%v", tc.ok, err)
			}
		})
	}
}

func TestSourceBlockObligationRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source *Source
		ok     bool
	}{
		{"none without a recipe", &Source{Obligation: "none", Basis: "scanned", Licenses: []string{"MIT"}}, true},
		{"none with an upstream only", &Source{Obligation: "none", Basis: "declared", Licenses: []string{"MIT"}, Upstream: validSource().Upstream}, true},
		{"none with licenses missing", &Source{Obligation: "none", Basis: "declared"}, false},
		{"undetermined with a recipe but no upstream", &Source{Obligation: "undetermined", Recipe: &SourceRecipe{Kind: "cargo-vendor"}}, false},
		{"undetermined with upstream and recipe", &Source{Obligation: "undetermined", Upstream: validSource().Upstream, Recipe: &SourceRecipe{Kind: "cargo-vendor"}}, true},
		{"undetermined alone", &Source{Obligation: "undetermined"}, true},
		{"undetermined with a bad basis", &Source{Obligation: "undetermined", Basis: "x"}, false},
		{"source-offer", func() *Source { s := validSource(); s.Obligation = "source-offer"; return s }(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := withSource(t, "shellcheck", tc.source); (err == nil) != tc.ok {
				t.Fatalf("ok=%v, err=%v", tc.ok, err)
			}
		})
	}
}

func TestSourceProductNames(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
		ok    bool
	}{
		{"plain names", []string{"fd", "rg", "git-lfs", "jsonschema-cli", "uv"}, true},
		{"digits first", []string{"7zip"}, true},
		{"longest allowed", []string{strings.Repeat("a", 32)}, true},
		{"too long", []string{strings.Repeat("a", 33)}, false},
		{"empty", []string{""}, false},
		{"upper case", []string{"Fish"}, false},
		{"underscore", []string{"git_lfs"}, false},
		{"dot", []string{"a.b"}, false},
		{"leading hyphen", []string{"-fd"}, false},
		{"slash", []string{"a/b"}, false},
		{"-v prefix of another", []string{"gh", "gh-v2"}, false},
		{"-v prefix in either order", []string{"gh-v2", "gh"}, false},
		{"hyphen without v is not a prefix", []string{"gh", "gh-cli", "gh-x"}, true},
		{"shared stem only", []string{"git", "git-lfs", "git-cliff"}, true},
		{"v inside a longer word", []string{"git", "git-value"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateProductNames(tc.names); (err == nil) != tc.ok {
				t.Fatalf("ok=%v, err=%v", tc.ok, err)
			}
		})
	}
}

func TestCatalogRefusesPrefixRelatedProductNames(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	// The real catalog satisfies the rule; a clone named gh-v2 breaks it.
	c.Catalog.Packages["gh-v2"] = c.Catalog.Packages["gh"]
	c.Desired["gh-v2"] = c.Desired["gh"]
	for _, arch := range []string{"amd64", "arm64"} {
		c.Lock.Platforms[arch]["gh-v2"] = c.Lock.Platforms[arch]["gh"]
	}
	c.Bind()
	err = c.Validate()
	if err == nil || !strings.Contains(err.Error(), "product") {
		t.Fatalf("accepted prefix-related product names: %v", err)
	}
}

func TestSourceBlockAcceptsVersionOnlyTemplates(t *testing.T) {
	for _, tarball := range []string{
		"https://static.crates.io/crates/x/x-{version}.crate",
		"https://github.com/a/b/archive/refs/tags/{lock_version}.tar.gz",
		"https://api.github.com/repos/a/b/tarball/v{version}",
	} {
		s := validSource()
		s.Upstream.Tarball = tarball
		if err := withSource(t, "shellcheck", s); err != nil {
			t.Errorf("%s: %v", tarball, err)
		}
	}
}
