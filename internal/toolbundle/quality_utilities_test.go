package toolbundle

import (
	"bundledock/internal/toolconfig"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Exercise the real installer with synthetic payloads, never upstream binaries.
func TestQualityUtilitiesInstallExecutablesAndFullNotices(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			payloads := map[string][]byte{}
			packages := map[string]toolconfig.Package{}
			specs := map[string]Spec{}
			for _, name := range []string{"shellcheck", "shfmt", "just", "ruff", "delta", "watchexec", "hyperfine", "age", "leaf", "jj", "lazygit", "git-lfs"} {
				p := c.Catalog.Packages[name]
				packages[name] = p
				var files []member
				for executable := range p.Install.Members {
					files = append(files, member{"upstream/" + executable, "synthetic " + executable, 0755, 0})
				}
				for _, notice := range p.Install.ArchiveNotices {
					files = append(files, member{"upstream/" + notice, "full upstream " + notice, 0644, 0})
				}
				switch p.Install.Kind {
				case "raw":
					payloads[name] = []byte("synthetic executable")
				case "tar.xz":
					payloads[name] = xzArchive(t, files...)
				default:
					payloads[name] = archive(t, files...)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payloads[r.URL.Path[1:]]) }))
			defer server.Close()
			for name, payload := range payloads {
				specs[name] = Spec{Version: c.Lock.Platforms[arch][name].Version, URL: server.URL + "/" + name, SHA256: fmt.Sprintf("%x", sha256.Sum256(payload))}
			}
			dst := t.TempDir()
			if err := Build(context.Background(), server.Client(), specs, packages, os.DirFS("../../bundles/toolbox"), arch, dst, io.Discard); err != nil {
				t.Fatal(err)
			}
			for name, p := range packages {
				for _, executable := range p.Install.Members {
					info, err := os.Stat(filepath.Join(dst, executable))
					if err != nil || info.Mode().Perm() != 0755 {
						t.Fatalf("%s: %v %v", executable, info, err)
					}
				}
				notices := append([]string{}, p.Install.ArchiveNotices...)
				for _, source := range p.Notices {
					notices = append(notices, filepath.Base(source))
				}
				for _, notice := range notices {
					data, err := readInstalledFile(t, dst, "licenses/"+name+"/"+notice)
					if err != nil || len(data) == 0 {
						t.Fatalf("%s/%s: %v", name, notice, err)
					}
				}
			}
		})
	}
}
