package toolpublish

import (
	"path/filepath"
	"testing"
)

// withoutNixbox drops the nixbox bundle's files from a bundles/* glob: it is a
// Nix store bundle with its own catalog and lock format (internal/nixbox), not
// a tool-bundle catalog.
func withoutNixbox(paths []string) []string {
	var kept []string
	for _, p := range paths {
		if filepath.Base(filepath.Dir(p)) != "nixbox" {
			kept = append(kept, p)
		}
	}
	return kept
}

func TestWithoutNixboxDropsOnlyNixbox(t *testing.T) {
	got := withoutNixbox([]string{"b/nixbox/packages.lock.json", "b/node/packages.lock.json", "b/toolbox/nixbox.json"})
	if len(got) != 2 || got[0] != "b/node/packages.lock.json" {
		t.Fatalf("%v", got)
	}
}
