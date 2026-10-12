package toolpublish

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLegacyArtifactUsesExactAttemptAndCredentialFreeDownload(t *testing.T) {
	for _, scenario := range []string{"legacy tag", "current tag", "package filename", "mixed filenames", "expired", "wrong revision", "download gone", "unsafe download", "duplicate lock"} {
		t.Run(scenario, func(t *testing.T) {
			lock := testLock(t)
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			filename := "tools.lock.json"
			if scenario == "package filename" {
				filename = "packages.lock.json"
			}
			f, _ := writer.Create(filename)
			f.Write(lock)
			if scenario == "duplicate lock" || scenario == "mixed filenames" {
				duplicate := filename
				if scenario == "mixed filenames" {
					duplicate = "packages.lock.json"
				}
				f, _ = writer.Create(duplicate)
				f.Write(lock)
			}
			writer.Close()
			m := Metadata{Image: "quay.io/example/toolbox", RunID: "38072919085", Tag: "2026.10.10-38072919085-4", Commit: strings.Repeat("a", 40), Source: "https://github.com/example/repo"}
			if scenario == "current tag" {
				m.Tag = "2026.10.10-b12.4"
			}
			downloads := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body, status := "", http.StatusOK
				headers := make(http.Header)
				if r.URL.Host == "artifact-download.example" {
					downloads++
					if r.Header.Get("Authorization") != "" {
						t.Fatal("forwarded GitHub token to signed download")
					}
					body = archive.String()
				} else {
					if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer private-token" {
						t.Fatal("unexpected token destination")
					}
					switch r.URL.Path {
					case "/repos/example/repo/actions/runs/38072919085/attempts/4":
						commit := m.Commit
						if scenario == "wrong revision" {
							commit = strings.Repeat("b", 40)
						}
						body = fmt.Sprintf(`{"id":38072919085,"run_attempt":4,"head_sha":%q,"head_branch":"main","conclusion":"success"}`, commit)
					case "/repos/example/repo/actions/runs/38072919085/artifacts":
						body = fmt.Sprintf(`{"artifacts":[{"id":10,"name":"resolved-tools-4","expired":%t}]}`, scenario == "expired")
					case "/repos/example/repo/actions/artifacts/10/zip":
						status = http.StatusFound
						headers.Set("Location", "https://artifact-download.example/signed?private-signature=value")
						if scenario == "download gone" {
							status = http.StatusGone
						}
						if scenario == "unsafe download" {
							headers.Set("Location", "http://artifact-download.example/")
						}
					default:
						t.Fatalf("unexpected artifact query %s", r.URL)
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers}, nil
			})}
			got, err := PublishedLockArtifact(context.Background(), client, "example/repo", "main", "private-token", m)
			wantErr := scenario != "legacy tag" && scenario != "current tag" && scenario != "package filename"
			if (err != nil) != wantErr || (!wantErr && (!bytes.Equal(got, lock) || downloads != 1)) {
				t.Fatalf("got %v downloads=%d", err, downloads)
			}
			if err != nil && strings.Contains(err.Error(), "private-") {
				t.Fatal("error exposes credentials or signed location")
			}
		})
	}
}
