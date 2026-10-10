package toolbundle

import (
	"bundledock/internal/toolconfig"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"testing"
)

// Optional release acceptance check: downloaded archives must match locked
// hashes and work with the same extractor used by the OCI build.
func TestQualitySecurityOfficialArchives(t *testing.T) {
	evidence := os.Getenv("BUNDLEDOCK_ARCHIVE_DIR")
	if evidence == "" {
		t.Skip("official archive directory not supplied")
	}
	c, err := toolconfig.Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(evidence)))
	defer server.Close()
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			packages := map[string]toolconfig.Package{}
			specs := map[string]Spec{}
			for _, name := range []string{"typos", "lychee", "gitleaks", "trivy", "syft"} {
				packages[name] = c.Catalog.Packages[name]
				spec := c.Lock.Platforms[arch][name]
				u, err := url.Parse(spec.URL)
				if err != nil {
					t.Fatal(err)
				}
				spec.URL = server.URL + "/" + path.Base(u.Path)
				specs[name] = spec
			}
			dst := t.TempDir()
			if err := Build(context.Background(), server.Client(), specs, packages, os.DirFS("../.."), arch, dst, io.Discard); err != nil {
				t.Fatal(err)
			}
			for name := range packages {
				info, err := os.Stat(filepath.Join(dst, "bin", name))
				if err != nil || info.Mode().Perm() != 0755 {
					t.Fatalf("%s executable: %v %v", name, info, err)
				}
				for _, notice := range packages[name].Install.ArchiveNotices {
					info, err := os.Stat(filepath.Join(dst, "licenses", name, notice))
					if err != nil || info.Size() == 0 {
						t.Fatalf("%s notice: %v", name, err)
					}
				}
			}
		})
	}
}
