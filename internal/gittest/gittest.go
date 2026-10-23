// Package gittest holds helpers for tests that create throwaway Git repositories.
package gittest

import (
	"os"
	"path/filepath"
	"testing"
)

// NoMaintenanceConfig is the content of a Git config file that stops background
// maintenance. Git detaches "maintenance run --auto" after commits and
// "gc --auto" after receives; under the geometric strategy it repacks into
// objects/pack depending on the commit hash, outliving the command and racing
// t.TempDir cleanup (ENOTEMPTY). A file is needed because receive-pack clears
// command-scope configuration inherited from push; receive.autoGC alone governs
// receive-pack's gc on older Git.
const NoMaintenanceConfig = "[maintenance]\n\tauto = false\n[gc]\n\tauto = 0\n[receive]\n\tautoGC = false\n"

// WriteGlobalConfig writes NoMaintenanceConfig to path for use as GIT_CONFIG_GLOBAL.
func WriteGlobalConfig(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(NoMaintenanceConfig), 0o600); err != nil {
		t.Fatal(err)
	}
}

// GlobalConfig writes the config into a fresh temporary directory and returns its path.
func GlobalConfig(t testing.TB) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gitconfig")
	WriteGlobalConfig(t, path)
	return path
}
