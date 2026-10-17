package toolconfig

import (
	"strings"
	"testing"
)

func TestSourceVersionShape(t *testing.T) {
	for _, tc := range []struct {
		name, regex, lock, want string
		ok                      bool
	}{
		{"v prefix", "", "v0.11.0", "0.11.0", true},
		{"bare", "", "0.51.0", "0.51.0", true},
		{"leading zeros inside a group", "", "25.07.1", "25.07.1", true},
		{"one group", "", "v3", "3", true},
		{"four groups", "", "1.2.3.4", "1.2.3.4", true},
		{"nine digits", "", "123456789.0.0", "123456789.0.0", true},
		{"ten digits", "", "1234567890.0.0", "", false},
		{"five groups", "", "1.2.3.4.5", "", false},
		{"tool prefix needs a regex", "", "lychee-v0.24.2", "", false},
		{"letter suffix", "", "v3.7c", "", false},
		{"prerelease", "", "v1.2.3-rc.1", "", false},
		{"empty", "", "", "", false},
		{"regex for a tool prefix", `^lychee-v([0-9.]+)$`, "lychee-v0.24.2", "0.24.2", true},
		{"regex for jq", `^jq-([0-9.]+)$`, "jq-1.8.2", "1.8.2", true},
		{"regex does not match", `^jq-([0-9.]+)$`, "v1.8.2", "", false},
		{"regex yields a bad shape", `^(.*)$`, "v3.7c", "", false},
		{"regex capture ten digits", `^x([0-9]+)$`, "x1234567890", "", false},
		{"unanchored regex spanning the whole version", `lychee-v([0-9.]+)`, "lychee-v0.24.2", "0.24.2", true},
		{"regex must span the whole version: prerelease", `([0-9.]+)`, "1.2.3-rc1", "", false},
		{"regex must span the whole version: prefix", `([0-9.]+)`, "x1.2y", "", false},
		{"regex must span the whole version: suffix", `v([0-9.]+)`, "v1.2.3-rc1", "", false},
		{"trailing newline with the default rule", "", "v1.2.3\n", "", false},
		{"trailing newline with a regex", `^lychee-v([0-9.]+)$`, "lychee-v0.24.2\n", "", false},
		{"regex yields five groups", `^x(.*)$`, "x1.2.3.4.5", "", false},
		{"two capture groups with an optional second", `([0-9.]+)(-rc)?`, "1.2-rc", "", false},
		{"regex capture empty", `^x([0-9]*)$`, "x", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Package{Source: &Source{Obligation: "source-offer", VersionRegex: tc.regex}}
			got, err := p.SourceVersion(tc.lock)
			if (err == nil) != tc.ok || got != tc.want {
				t.Fatalf("got %q, err=%v", got, err)
			}
		})
	}
	// Without a source block the default rule applies.
	if got, err := (Package{}).SourceVersion("v1.2.3"); err != nil || got != "1.2.3" {
		t.Fatalf("got %q, %v", got, err)
	}
}

// version-shape is emitted by pin and plan (spec.md, failure modes), never by
// catalog validation: check stays report-only until WI8, so a lock version
// without a numeric shape must not make Load, upgrade or publish fail.
func TestValidateDoesNotEnforceVersionShape(t *testing.T) {
	for _, name := range []string{"lychee", "tmux"} {
		if err := withSource(t, name, validSource()); err != nil {
			t.Fatalf("%s blocked by version shape: %v", name, err)
		}
	}
	s := validSource()
	s.VersionRegex = `^never-matches-([0-9.]+)$`
	if err := withSource(t, "shellcheck", s); err != nil {
		t.Fatalf("a regex that does not match the lock version blocked validation: %v", err)
	}
}

func TestSourceVersionInvalidRegexNeverPanics(t *testing.T) {
	for name, regex := range map[string]string{"no group": `^v[0-9.]+$`, "two groups": `^(v)([0-9.]+)$`, "does not compile": "([0-9"} {
		t.Run(name, func(t *testing.T) {
			p := Package{Source: &Source{Obligation: "source-offer", VersionRegex: regex}}
			if v, err := p.SourceVersion("v1.2.3"); err == nil || v != "" {
				t.Fatalf("got %q, %v", v, err)
			}
		})
	}
}

// The whole lock version is matched, so alternation and lazy quantifiers
// cannot make a valid full match fail on a shorter leftmost match.
func TestSourceVersionFullMatchSearchesAlternatives(t *testing.T) {
	for _, tc := range []struct{ regex, lock, want string }{
		{`v?([0-9.]+?)`, "v1.2.3", "1.2.3"},
		{`(1|1\.2)`, "1.2", "1.2"},
		{`^(1|1\.2)$`, "1.2", "1.2"},
		{`x?(1\.2|1)(?:\.3)?`, "x1.2.3", "1.2"},
	} {
		p := Package{Source: &Source{Obligation: "source-offer", VersionRegex: tc.regex}}
		if got, err := p.SourceVersion(tc.lock); err != nil || got != tc.want {
			t.Errorf("%s on %s: got %q, %v", tc.regex, tc.lock, got, err)
		}
	}
	// A user pattern with an alternation at top level must not escape the anchors.
	p := Package{Source: &Source{Obligation: "source-offer", VersionRegex: `([0-9.]+)|x`}}
	if got, err := p.SourceVersion("1.2.3-rc1"); err == nil {
		t.Fatalf("got %q", got)
	}
}

func TestShortenBoundsEchoedValues(t *testing.T) {
	long := strings.Repeat("9", 500)
	_, err := (Package{}).SourceVersion(long)
	if err == nil || len(err.Error()) > 200 || !strings.Contains(err.Error(), "...") || strings.Contains(err.Error(), long) {
		t.Fatalf("echo not truncated: %v", err)
	}
	if got := shorten("short"); got != "short" {
		t.Fatalf("short value changed: %q", got)
	}
	if got := shorten(strings.Repeat("a", 41)); got != strings.Repeat("a", 40)+"..." {
		t.Fatalf("41 byte value: %q", got)
	}
	if got := shorten(strings.Repeat("a", 40)); got != strings.Repeat("a", 40) {
		t.Fatalf("40 byte value truncated: %q", got)
	}
}
