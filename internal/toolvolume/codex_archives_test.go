package toolvolume

import (
	"bundledock/internal/toolbundle"
	"bundledock/internal/toolconfig"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Optional acceptance uses complete official archives through the production
// checksum verifier, extractor and volume copier. Only native binaries execute.
func TestCodexOfficialArchivesWithoutVoice(t *testing.T) {
	evidence := os.Getenv("BUNDLEDOCK_CODEX_ARCHIVE_DIR")
	if evidence == "" {
		t.Skip("official archive directory not supplied")
	}
	c, err := toolconfig.Load("../../bundles/codex")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(evidence)))
	defer server.Close()
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			spec := c.Lock.Platforms[arch]["codex"]
			spec.URL = server.URL + "/" + path.Base(spec.URL)
			source := t.TempDir()
			if err := toolbundle.Build(context.Background(), server.Client(), map[string]toolconfig.Spec{"codex": spec}, map[string]toolconfig.Package{"codex": c.Catalog.Packages["codex"]}, os.DirFS("../../bundles/codex"), arch, source, io.Discard); err != nil {
				t.Fatal(err)
			}
			destination := t.TempDir()
			if err := Initialize(source, destination, io.Discard); err != nil {
				t.Fatal(err)
			}
			for _, dir := range []string{source, destination} {
				absent(t, dir, "codex-resources/voice")
				for _, name := range []string{"bin/codex", "bin/codex-code-mode-host", "codex-path/rg", "codex-resources/bwrap", "codex-package.json"} {
					info, err := os.Stat(filepath.Join(dir, name))
					if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
						t.Fatalf("native package resource %s: %v %v", name, info, err)
					}
				}
				if _, err := os.Stat(filepath.Join(dir, "codex-resources/zsh")); err == nil {
					info, err := os.Stat(filepath.Join(dir, "codex-resources/zsh/bin/zsh"))
					if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
						t.Fatalf("native package resource codex-resources/zsh/bin/zsh: %v %v", info, err)
					}
				}
			}
			if runtime.GOOS == "linux" && runtime.GOARCH == arch && os.Getenv("BUNDLEDOCK_CODEX_NATIVE_SMOKE") == "1" {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("CODEX_HOME", filepath.Join(os.Getenv("HOME"), ".codex"))
				if err := os.MkdirAll(os.Getenv("CODEX_HOME"), 0700); err != nil {
					t.Fatal(err)
				}
				binary := filepath.Join(destination, "bin/codex")
				run := func(args ...string) string {
					t.Helper()
					ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
					defer cancel()
					output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
					if err != nil {
						t.Fatalf("codex %v: %v\n%s", args, err, output)
					}
					t.Logf("codex %v: %s", args, strings.TrimSpace(string(output)))
					return string(output)
				}
				version, err := c.Catalog.Packages["codex"].Version(spec.Version)
				if err != nil {
					t.Fatal(err)
				}
				if output := run("--version"); !strings.Contains(output, version) {
					t.Fatalf("unexpected version %q", output)
				}
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = exec.CommandContext(ctx, binary, "app-server", "daemon", "stop").Run()
				})
				run("app-server", "daemon", "start")
				run("app-server", "daemon", "version")
				run("app-server", "daemon", "stop")
			}
		})
	}
}
