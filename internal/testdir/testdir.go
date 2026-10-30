// Package testdir provides temporary directories whose paths contain no
// symbolic links.
package testdir

import (
	"path/filepath"
	"testing"
)

// New returns a fresh temporary directory like t.TempDir, but with symbolic
// links in its path resolved. On macOS the temp root lives under /var and
// /tmp, which are links to /private/..., so scripts that refuse linked path
// components or compare absolute paths misbehave with the raw path. Where the
// temp root is not linked (Linux) the result is identical to t.TempDir.
func New(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolve temp dir %s: %v", dir, err)
	}
	return resolved
}
