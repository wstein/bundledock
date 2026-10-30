package toolbundle

import (
	"bundledock/internal/testdir"
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
	// The glibc-dynamic release binaries are pinned and noticed in the nixbox tools directory.
	for _, set := range []struct {
		directory string
		names     []string
	}{
		{"../../bundles/toolbox", []string{"shellcheck", "shfmt", "just", "ruff", "watchexec", "hyperfine", "age", "jj", "lazygit", "git-lfs", "navi", "typst", "trufflehog", "lsd", "actionlint", "hadolint", "pinact", "cosign", "git-cliff", "zoxide", "jsonschema-cli", "gh-stack"}},
		{"../../bundles/nixbox/tools", []string{"delta", "leaf", "zizmor", "difftastic"}},
	} {
		t.Run(filepath.Base(set.directory), func(t *testing.T) { installAndCheckNotices(t, set.directory, set.names) })
	}
}

func installAndCheckNotices(t *testing.T, directory string, names []string) {
	c, err := toolconfig.Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			payloads := map[string][]byte{}
			packages := map[string]toolconfig.Package{}
			specs := map[string]Spec{}
			for _, name := range names {
				p := c.Catalog.Packages[name]
				packages[name] = p
				var files []member
				for executable := range p.Install.Members {
					files = append(files, member{"upstream/" + executable, "synthetic " + executable, 0755, 0})
				}
				for _, notice := range p.Install.ArchiveNotices {
					files = append(files, member{"upstream/" + notice, "full upstream " + notice, 0644, 0})
				}
				for _, tree := range p.Install.ArchiveNoticeTrees {
					files = append(files, member{tree + "/pkg/LICENSE", "tree license", 0644, 0})
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
			dst := testdir.New(t)
			if err := Build(context.Background(), server.Client(), specs, packages, os.DirFS(directory), arch, dst, io.Discard); err != nil {
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
