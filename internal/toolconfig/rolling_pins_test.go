package toolconfig

import (
	"strings"
	"testing"
)

// The desired input defines allowed stable releases; mutable locks must not be
// compared with a historical version or checksum copied into a test.
func assertRollingStablePins(t *testing.T, c *Config, name string) {
	t.Helper()
	p := c.Catalog.Packages[name]
	previous := ""
	for _, arch := range []string{"amd64", "arm64"} {
		pin := c.Lock.Platforms[arch][name]
		version, err := p.Version(pin.Version)
		if err != nil || !c.Desired[name].Contains(version) {
			t.Fatalf("%s/%s is not a stable release within desired constraints: %q", name, arch, pin.Version)
		}
		if err := ValidateSpec(pin); err != nil || len(pin.SHA256) != 64 {
			t.Fatalf("%s/%s lacks a validated SHA256 pin: %v", name, arch, err)
		}
		if previous != "" && previous != version {
			t.Fatalf("%s architecture versions differ", name)
		}
		previous = version
		if p.Provider == "github" && !strings.HasPrefix(pin.URL, "https://github.com/"+p.Repository+"/releases/download/"+pin.Version+"/") {
			t.Fatalf("%s/%s does not use an official versioned release URL", name, arch)
		}
	}
}

// Retain stronger publisher-checksum contracts where the recipe tests already
// require them; do not impose a new freeze on allowed download-hash fallbacks.
func assertRollingPublisherPins(t *testing.T, c *Config, name string) {
	t.Helper()
	assertRollingStablePins(t, c, name)
	for _, arch := range []string{"amd64", "arm64"} {
		if c.Lock.Platforms[arch][name].Verification != "publisher-sha256" {
			t.Fatalf("%s/%s lacks publisher SHA256 provenance", name, arch)
		}
	}
}
