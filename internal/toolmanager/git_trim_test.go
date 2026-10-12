package toolmanager

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func TestGitTrimUpdaterNeverExecutesNewSourceRecipes(t *testing.T) {
	for _, tag := range []string{"v0.4.4", "v0.4.3"} {
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/foriequal0/git-trim/git/ref/tags/v0.4.3" {
				t.Fatalf("unexpected download: %s", r.URL)
			}
			fmt.Fprint(w, `{"object":{"type":"commit","sha":"0000000000000000000000000000000000000000"}}`)
		})
		if _, err := (resolver{client}).gitTrim(context.Background(), release{Tag: tag}); err == nil {
			t.Fatal("unreviewed source accepted")
		}
	}
}

func TestGitTrimArchiveIntegrity(t *testing.T) {
	lock, err := os.ReadFile("testdata/git-trim-Cargo.lock")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "changed lock", "traversal", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			for n, b := range map[string][]byte{"Cargo.lock": lock, "Cargo.toml": []byte("name = \"git-trim\"\nversion = \"0.4.3\"\n"), "LICENSE": []byte("MIT")} {
				if scenario == "changed lock" && n == "Cargo.lock" {
					b = []byte("changed")
				}
				h := &tar.Header{Name: "git-trim-" + toolconfig.GitTrimCommit + "/" + n, Size: int64(len(b)), Mode: 0644}
				if n == "LICENSE" && scenario == "traversal" {
					h.Name = "git-trim-" + toolconfig.GitTrimCommit + "/../escape"
				}
				if n == "LICENSE" && scenario == "symlink" {
					h.Typeflag = tar.TypeSymlink
					h.Linkname = "/etc/passwd"
					h.Size = 0
					b = nil
				}
				if err := tw.WriteHeader(h); err != nil {
					t.Fatal(err)
				}
				if _, err := tw.Write(b); err != nil {
					t.Fatal(err)
				}
			}
			tw.Close()
			gz.Close()
			_, err := toolconfig.ReadGitTrimArchive(buf.Bytes(), fmt.Sprintf("%x", sha256.Sum256(buf.Bytes())))
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("archive result %v", err)
			}
			if _, err := toolconfig.ReadGitTrimArchive(buf.Bytes(), toolconfig.GitTrimArchiveSHA256); err == nil {
				t.Fatal("unverified source accepted")
			}
		})
	}
}
