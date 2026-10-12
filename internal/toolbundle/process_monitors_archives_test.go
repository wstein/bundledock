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
func TestProcessMonitorsOfficialArchives(t *testing.T) {
	evidence := os.Getenv("BUNDLEDOCK_PROCESS_MONITORS_ARCHIVE_DIR")
	if evidence == "" {
		t.Skip("official archive directory not supplied")
	}
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(evidence)))
	defer server.Close()
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			packages := map[string]toolconfig.Package{}
			specs := map[string]Spec{}
			for _, name := range []string{"bottom", "btop"} {
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
			if err := Build(context.Background(), server.Client(), specs, packages, os.DirFS("../../bundles/toolbox"), arch, dst, io.Discard); err != nil {
				t.Fatal(err)
			}
			for name := range packages {
				info, err := os.Stat(filepath.Join(dst, "bin", map[string]string{"bottom": "btm", "btop": "btop"}[name]))
				if err != nil || info.Mode().Perm() != 0755 {
					t.Fatalf("%s executable: %v %v", name, info, err)
				}
				notices := append([]string{}, packages[name].Install.ArchiveNotices...)
				for _, source := range packages[name].Notices {
					notices = append(notices, filepath.Base(source))
				}
				for _, notice := range notices {
					data, err := readInstalledFile(t, dst, "licenses/"+name+"/"+notice)
					if err != nil || len(data) == 0 {
						t.Fatalf("%s notice: %v", name, err)
					}
				}
			}
		})
	}
}
