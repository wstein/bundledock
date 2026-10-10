package toolbundle

import (
	"bundledock/internal/toolconfig"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Optional acceptance check: verify both official archives with the OCI extractor,
// then execute the extracted native binary, never a Hugo found on the host PATH.
func TestHugoOfficialArchives(t *testing.T) {
	evidence := os.Getenv("BUNDLEDOCK_HUGO_ARCHIVE_DIR")
	if evidence == "" {
		t.Skip("official Hugo archive directory not supplied")
	}
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(evidence)))
	defer server.Close()
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			p := c.Catalog.Packages["hugo"]
			spec := c.Lock.Platforms[arch]["hugo"]
			u, err := url.Parse(spec.URL)
			if err != nil {
				t.Fatal(err)
			}
			spec.URL = server.URL + "/" + path.Base(u.Path)
			dst := t.TempDir()
			if err := Build(context.Background(), server.Client(), map[string]Spec{"hugo": spec}, map[string]toolconfig.Package{"hugo": p}, os.DirFS("../../bundles/toolbox"), arch, dst, io.Discard); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dst, "bin", "hugo")
			info, err := os.Stat(binary)
			if err != nil || info.Mode().Perm() != 0755 {
				t.Fatalf("Hugo executable: %v %v", info, err)
			}
			archiveInfo, err := os.Stat(filepath.Join(evidence, path.Base(u.Path)))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s archive=%d binary=%d bytes", arch, archiveInfo.Size(), info.Size())
			for _, notice := range []string{"LICENSE", "SOURCE"} {
				info, err := os.Stat(filepath.Join(dst, "licenses", "hugo", notice))
				if err != nil || info.Size() == 0 {
					t.Fatalf("Hugo %s: %v", notice, err)
				}
			}
			if runtime.GOOS != "linux" || runtime.GOARCH != arch {
				return
			}
			data, err := exec.Command(binary, "version").CombinedOutput()
			if err != nil || !strings.HasPrefix(string(data), "hugo "+spec.Version+"-") || strings.Contains(string(data), "extended") {
				t.Fatalf("standard pinned Hugo version: %v: %s", err, data)
			}
			t.Log(strings.TrimSpace(string(data)))
			home := t.TempDir()
			smoke := exec.Command("sh", "../../bundles/toolbox/hugo-smoke.sh", binary)
			smoke.Env = append(os.Environ(), "HOME="+home)
			if data, err := smoke.CombinedOutput(); err != nil {
				t.Fatalf("offline minimal docs build: %v: %s", err, data)
			}
			root, err := filepath.Abs("../..")
			if err != nil {
				t.Fatal(err)
			}
			output := t.TempDir()
			docs := exec.Command(binary, "--source", root, "--destination", output, "--cacheDir", filepath.Join(home, ".cache", "hugo"), "--noBuildLock", "--baseURL", "https://example.invalid/bundledock/", "--panicOnWarning")
			docs.Env = append(os.Environ(), "HOME="+home, "HUGO_RESOURCEDIR="+filepath.Join(home, "resources"))
			if data, err := docs.CombinedOutput(); err != nil {
				t.Fatalf("current BundleDock docs build: %v: %s", err, data)
			}
			for page, text := range map[string]string{"index.html": "Build once. Share your tools.", "guides/tools/index.html": "Hugo", "guides/development/index.html": "Verification evidence"} {
				data, err := os.ReadFile(filepath.Join(output, page))
				if err != nil || !strings.Contains(string(data), text) {
					t.Fatalf("rendered %s missing %q: %v", page, text, err)
				}
			}
		})
	}
}
