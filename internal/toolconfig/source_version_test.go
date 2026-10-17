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

func TestCopyleftPackagesMustYieldANumericVersion(t *testing.T) {
	copyleft := func(regex string) *Source {
		s := validSource()
		s.VersionRegex = regex
		return s
	}
	if err := withSource(t, "lychee", copyleft("")); err == nil || !strings.Contains(err.Error(), "version-shape") {
		t.Fatalf("lychee-v0.24.2 accepted without a regex: %v", err)
	}
	if err := withSource(t, "lychee", copyleft(`^lychee-v([0-9.]+)$`)); err != nil {
		t.Fatalf("regex rejected: %v", err)
	}
	if err := withSource(t, "tmux", copyleft("")); err == nil || !strings.Contains(err.Error(), "version-shape") {
		t.Fatalf("tmux v3.7c accepted: %v", err)
	}
	// Classes that need no mirror release are not blocked by the version shape.
	for _, s := range []*Source{{Obligation: "undetermined"}, {Obligation: "none", Basis: "declared", Licenses: []string{"MIT"}}} {
		if err := withSource(t, "tmux", s); err != nil {
			t.Fatalf("%s blocked: %v", s.Obligation, err)
		}
	}
}
