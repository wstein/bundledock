package toolconfig

import "testing"

func TestBundledBashRemovedAndAlternativesPreserved(t *testing.T) {
	c, err := Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Catalog.Packages["bash"]; ok {
		t.Fatal("bundled Bash remains in catalog")
	}
	if _, ok := c.Desired["bash"]; ok {
		t.Fatal("bundled Bash remains desired")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if _, ok := c.Lock.Platforms[arch]["bash"]; ok {
			t.Fatal("bundled Bash remains locked")
		}
	}
	for _, name := range []string{"nu", "tmux", "fish", "herdr", "starship"} {
		if _, ok := c.Catalog.Packages[name]; !ok {
			t.Fatalf("preferred tool or alternative %s removed", name)
		}
	}
}
