package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHugoRejectsCorruptCachedArchive(t *testing.T) {
	if os.Getenv("BUNDLEDOCK_DOCS_TEST") != "1" {
		t.Skip("host Hugo integration check; run make docs-test to enable")
	}
	cache := t.TempDir()
	// Both supported platform names are populated, so this test never downloads.
	for _, arch := range []string{"amd64", "arm64"} {
		if err := os.WriteFile(filepath.Join(cache, "hugo_0.167.0_linux-"+arch+".tar.gz"), []byte("corrupt archive"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "scripts/hugo.sh", "version")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "HUGO_CACHE_DIR="+cache)
	data, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(data), "checksum") {
		t.Fatalf("expected checksum rejection, got %v: %s", err, data)
	}
}
