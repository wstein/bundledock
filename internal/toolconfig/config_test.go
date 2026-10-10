package toolconfig

import (
	"strings"
	"testing"
)

func TestDesiredBoundedStableVersions(t *testing.T) {
	desired, err := ParseDesired(strings.NewReader("# tools\ncodex >=0.162.1,<0.163.0 # compatible\ngh==2.102.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, version string
		want          bool
	}{
		{"codex", "0.162.1", true}, {"codex", "0.162.9", true}, {"codex", "0.163.0", false}, {"codex", "0.162.2-rc.1", false},
		{"gh", "2.102.0", true}, {"gh", "2.102.1", false},
	} {
		if got := desired[tc.name].Contains(tc.version); got != tc.want {
			t.Errorf("%s %s: %v", tc.name, tc.version, got)
		}
	}
	for _, input := range []string{"gh>=2.0.0", "gh<3.0.0", "gh>=3.0.0,<2.0.0", "gh~=2.0", "gh==v2.0.0", "gh==02.0.0", "gh==2.0.0\ngh==2.0.0", "gh>=2.0.0, <3.0.0, ==2.1.0", "gh==2.0.0 extra", "gh==2.0.0-beta"} {
		if _, err := ParseDesired(strings.NewReader(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestRepositoryIdentityAndBothArchitectures(t *testing.T) {
	c, err := Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog.Packages) != 12 {
		t.Fatal("catalog incomplete")
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	original := c.Lock.CatalogDigest
	c.Lock.CatalogDigest = strings.Repeat("a", 64)
	if err := c.Validate(); err == nil {
		t.Fatal("accepted changed catalog")
	}
	c.Lock.CatalogDigest = original
	s := c.Lock.Platforms["arm64"]["gh"]
	delete(c.Lock.Platforms["arm64"], "gh")
	if err := c.Validate(); err == nil {
		t.Fatal("accepted missing arm64 tool")
	}
	c.Lock.Platforms["arm64"]["gh"] = s
	s.Version = "v999.0.0"
	c.Lock.Platforms["arm64"]["gh"] = s
	if err := c.Validate(); err == nil {
		t.Fatal("accepted divergent/out of range version")
	}
}

func TestCatalogRejectsIncompleteAndUnsafeRecipes(t *testing.T) {
	for _, kind := range []string{"provider", "platform", "shell", "template", "member", "alias", "sqlite-kind", "claude-base", "sqlite-history", "normalization"} {
		t.Run(kind, func(t *testing.T) {
			c, err := Load("../..")
			if err != nil {
				t.Fatal(err)
			}
			name := "gh"
			if kind == "sqlite-history" || kind == "sqlite-kind" {
				name = "sqlite3"
			}
			if kind == "claude-base" {
				name = "claude"
			}
			p := c.Catalog.Packages[name]
			switch kind {
			case "provider":
				p.Provider = "shell"
			case "platform":
				delete(p.Architectures, "arm64")
			case "shell":
				p.Install.Kind = "exec"
			case "template":
				p.Asset = "tool-{command}.tar.gz"
			case "member":
				p.Install.Members["../bad"] = "bin/bad"
			case "alias":
				p.Install.Aliases = map[string]string{"bin/alias": "../outside"}
			case "sqlite-kind":
				p.Install.Kind = "tar.gz"
			case "claude-base":
				p.BaseURL = ""
			case "sqlite-history":
				p.HistoryURL = ""
			case "normalization":
				p.Normalization = "sqlite-compact"
			}
			c.Catalog.Packages[name] = p
			c.Bind()
			if err := c.Validate(); err == nil {
				t.Fatalf("accepted invalid %s recipe", kind)
			}
		})
	}
}
func TestStableNumericOrdering(t *testing.T) {
	a, _ := ParseVersion("2.10.0")
	b, _ := ParseVersion("2.9.99")
	if a.Compare(b) <= 0 {
		t.Fatal("lexical version ordering")
	}
	for _, v := range []string{"1.2", "1.2.3.4", "1.2.3+build", "18446744073709551616.0.0", "1.02.3", "v1.2.3"} {
		if _, err := ParseVersion(v); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
}
