package toolbundle

import (
	"bundledock/internal/toolconfig"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Optional source acceptance check: inspect the official Linux archives without
// executing their binaries or building an OCI image.
func TestTruffleHogOfficialArchives(t *testing.T) {
	dir := os.Getenv("BUNDLEDOCK_TRUFFLEHOG_ARCHIVE_DIR")
	if dir == "" {
		t.Skip("official TruffleHog archive directory not supplied")
	}
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			pin := c.Lock.Platforms[arch]["trufflehog"]
			archive, err := os.Open(filepath.Join(dir, filepath.Base(pin.URL)))
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			hash := sha256.New()
			if _, err := io.Copy(hash, archive); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", hash.Sum(nil)) != pin.SHA256 {
				t.Fatal("archive differs from official pin")
			}
			if _, err := archive.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			dst := t.TempDir()
			root, err := os.OpenRoot(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := Extract(root, "trufflehog", c.Catalog.Packages["trufflehog"], archive); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(dst, "bin/trufflehog"))
			if err != nil || info.Mode().Perm() != 0755 {
				t.Fatalf("missing executable: %v", err)
			}
			license, err := os.ReadFile(filepath.Join(dst, "licenses/trufflehog/LICENSE"))
			if err != nil || !strings.Contains(string(license), "GNU AFFERO GENERAL PUBLIC LICENSE") {
				t.Fatalf("missing full upstream license: %v", err)
			}
		})
	}
}
