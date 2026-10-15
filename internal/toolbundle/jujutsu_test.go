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

func TestJujutsuArchiveInstallsBinaryAndExactLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	const license = "synthetic full Apache-2.0 text including upstream notices\n"
	for _, arch := range []string{"amd64", "arm64"} {
		for _, broken := range []string{"", "binary", "checksum"} {
			t.Run(arch+"/"+broken, func(t *testing.T) {
				members := []member{{"./LICENSE", license, 0644, 0}, {"./README.md", "upstream readme", 0644, 0}}
				if broken != "binary" {
					members = append(members, member{"./jj", "synthetic jj", 0755, 0})
				}
				payload := archive(t, members...)
				sum := fmt.Sprintf("%x", sha256.Sum256(payload))
				if broken == "checksum" {
					sum = fmt.Sprintf("%064d", 0)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
				defer server.Close()
				dst := t.TempDir()
				spec := c.Lock.Platforms[arch]["jj"]
				spec.URL = server.URL
				spec.SHA256 = sum
				err := Build(context.Background(), server.Client(), map[string]Spec{"jj": spec}, map[string]toolconfig.Package{"jj": c.Catalog.Packages["jj"]}, os.DirFS("../../bundles/toolbox"), arch, dst, io.Discard)
				if broken != "" {
					if err == nil {
						t.Fatal("invalid upstream payload accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				data, err := readInstalledFile(t, dst, "licenses/jj/LICENSE")
				if err != nil || string(data) != license {
					t.Fatalf("full license not preserved: %v %q", err, data)
				}
				binary, err := os.Stat(filepath.Join(dst, "bin", "jj"))
				if err != nil || binary.Mode().Perm() != 0755 {
					t.Fatalf("binary mode: %v %v", binary, err)
				}
				if _, err := os.Stat(filepath.Join(dst, "bin", "README.md")); !os.IsNotExist(err) {
					t.Fatalf("unexpected executable payload: %v", err)
				}
			})
		}
	}
}

// The shared shell layer belongs to the toolbox only, never to a single-tool
// build such as this jj-only one.
func TestSingleToolBuildHasNoShellLayer(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	payload := archive(t, member{"./LICENSE", "license", 0644, 0}, member{"./jj", "synthetic jj", 0755, 0})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
	defer server.Close()
	spec := c.Lock.Platforms["amd64"]["jj"]
	spec.URL = server.URL
	spec.SHA256 = fmt.Sprintf("%x", sha256.Sum256(payload))
	dst := t.TempDir()
	if err := Build(context.Background(), server.Client(), map[string]Spec{"jj": spec}, map[string]toolconfig.Package{"jj": c.Catalog.Packages["jj"]}, os.DirFS("../../bundles/toolbox"), "amd64", dst, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "share/shell")); !os.IsNotExist(err) {
		t.Fatalf("single-tool build carries the shell layer: %v", err)
	}
}
