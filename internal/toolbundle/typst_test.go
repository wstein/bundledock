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

func TestTypstArchiveInstallsBinaryAndCompleteNotices(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	const license = "synthetic full Apache-2.0 text including upstream notices\n"
	const notice = "synthetic complete third-party notice\n"
	for _, arch := range []string{"amd64", "arm64"} {
		for _, broken := range []string{"", "binary", "checksum"} {
			t.Run(arch+"/"+broken, func(t *testing.T) {
				members := []member{{"typst-target/LICENSE", license, 0644, 0}, {"typst-target/NOTICE", notice, 0644, 0}, {"typst-target/README.md", "upstream readme", 0644, 0}}
				if broken != "binary" {
					members = append(members, member{"typst-target/typst", "synthetic typst", 0755, 0})
				}
				payload := xzArchive(t, members...)
				sum := fmt.Sprintf("%x", sha256.Sum256(payload))
				if broken == "checksum" {
					sum = fmt.Sprintf("%064d", 0)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
				defer server.Close()
				dst := t.TempDir()
				spec := c.Lock.Platforms[arch]["typst"]
				spec.URL = server.URL
				spec.SHA256 = sum
				err := Build(context.Background(), server.Client(), map[string]Spec{"typst": spec}, map[string]toolconfig.Package{"typst": c.Catalog.Packages["typst"]}, os.DirFS("../../bundles/toolbox"), arch, dst, io.Discard)
				if broken != "" {
					if err == nil {
						t.Fatal("invalid upstream payload accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				data, err := readInstalledFile(t, dst, "licenses/typst/LICENSE")
				if err != nil || string(data) != license {
					t.Fatalf("full license not preserved: %v %q", err, data)
				}
				data, err = readInstalledFile(t, dst, "licenses/typst/NOTICE")
				if err != nil || string(data) != notice {
					t.Fatalf("full notice not preserved: %v %q", err, data)
				}
				expected, err := os.ReadFile("../../bundles/toolbox/licenses/typst/ASSETS-NOTICE")
				if err != nil {
					t.Fatal(err)
				}
				data, err = readInstalledFile(t, dst, "licenses/typst/ASSETS-NOTICE")
				if err != nil || string(data) != string(expected) {
					t.Fatalf("embedded fonts notice not preserved: %v", err)
				}
				binary, err := os.Stat(filepath.Join(dst, "bin", "typst"))
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
