package toolbundle

import (
	"bundledock/internal/toolconfig"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareBashSourceVerifiesAllInputsBeforeExtraction(t *testing.T) {
	for _, failure := range []string{"", "source-checksum", "patch-checksum", "patch-order", "source-link", "unexpected-member"} {
		t.Run(failure, func(t *testing.T) {
			data := archive(t, member{"bash-5.3/configure", "configure source", 0755, 0}, member{"bash-5.3/patchlevel.h", "#define PATCHLEVEL 0", 0644, 0})
			if failure == "source-link" {
				data = archive(t, member{"bash-5.3/link", "", 0755, 50})
			}
			if failure == "unexpected-member" {
				data = archive(t, member{"outside/file", "bad", 0644, 0})
			}
			patch := []byte("official patch fixture")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, ".tar.gz") {
					w.Write(data)
				} else {
					w.Write(patch)
				}
			}))
			defer server.Close()
			s := Spec{Version: "5.3.2", URL: server.URL + "/bash-5.3.tar.gz", SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Verification: "tls-download-sha256"}
			for i := 1; i <= 2; i++ {
				s.Patches = append(s.Patches, toolconfig.Input{URL: fmt.Sprintf("%s/bash-5.3-patches/bash53-%03d", server.URL, i), SHA256: fmt.Sprintf("%x", sha256.Sum256(patch)), Verification: "tls-download-sha256"})
			}
			if failure == "source-checksum" {
				s.SHA256 = strings.Repeat("0", 64)
			}
			if failure == "patch-checksum" {
				s.Patches[1].SHA256 = strings.Repeat("0", 64)
			}
			if failure == "patch-order" {
				s.Patches[0], s.Patches[1] = s.Patches[1], s.Patches[0]
			}
			dst := t.TempDir()
			err := PrepareBashSource(context.Background(), server.Client(), s, dst)
			if failure != "" {
				if err == nil {
					t.Fatal("accepted invalid source inputs")
				}
				if strings.Contains(failure, "checksum") {
					if _, err := os.Stat(filepath.Join(dst, "source")); !os.IsNotExist(err) {
						t.Fatal("source extracted before all checksums passed")
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range []string{"source/configure", "source/patchlevel.h", "patches/bash53-001", "patches/bash53-002"} {
				if _, err := os.Stat(filepath.Join(dst, n)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
