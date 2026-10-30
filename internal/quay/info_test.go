package quay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sha(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }

// fakeRegistry serves a tag listing plus anonymous token, index, manifest and config.
func fakeRegistry(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	config := []byte(`{"os":"linux","architecture":"amd64","config":{"Labels":{"org.opencontainers.image.version":"2026.10.30-b3","org.opencontainers.image.revision":"abc"}}}`)
	manifest := []byte(`{"config":{"digest":"` + sha(config) + `"}}`)
	index := []byte(`{"manifests":[{"digest":"` + sha(manifest) + `","platform":{"os":"linux","architecture":"amd64"}},{"digest":"` + sha(manifest) + `","platform":{"os":"linux","architecture":"arm64"}}]}`)
	blobs := map[string][]byte{sha(config): config, sha(manifest): manifest, sha(index): index}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/auth":
			if r.Header.Get("Authorization") != "" {
				t.Error("anonymous token request sent credentials")
			}
			w.Write([]byte(`{"token":"tok"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/bundledock/toolbox/"):
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			parts := strings.Split(r.URL.Path, "/")
			if data, ok := blobs[parts[len(parts)-1]]; ok {
				w.Write(data)
				return
			}
			http.NotFound(w, r)
		case r.URL.Path == "/api/v1/repository/bundledock/toolbox/tag/":
			json.NewEncoder(w).Encode(map[string]any{"page": 1, "has_additional": false, "tags": []Tag{
				{Name: "2026.10.30-b3", ManifestDigest: sha(index), LastModified: "Wed, 28 Oct 2026 10:00:00 -0000"},
				{Name: "latest", ManifestDigest: sha(index), LastModified: "Wed, 28 Oct 2026 10:00:00 -0000"},
				{Name: "gone", ManifestDigest: "sha256:" + strings.Repeat("0", 64), LastModified: "Wed, 28 Oct 2026 10:00:00 -0000"},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, sha(index)
}

func TestInfo(t *testing.T) {
	srv, digest := fakeRegistry(t)
	defer srv.Close()
	b, _ := FindBundle("toolbox")
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	rows, err := Info(context.Background(), New(srv.Client(), srv.URL), b, now)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows = %v, err = %v", rows, err)
	}
	var version InfoRow
	for _, r := range rows {
		if r.Tag == "2026.10.30-b3" {
			version = r
		}
	}
	if version.Digest != digest || version.AgeDays != 2 || strings.Join(version.Architectures, ",") != "linux/amd64,linux/arm64" || version.Version != "2026.10.30-b3" || version.Revision != "abc" {
		t.Fatalf("version row = %+v", version)
	}
	for _, r := range rows {
		if r.Tag == "gone" && r.Error == "" {
			t.Fatal("missing manifest not reported")
		}
	}
}

func TestBlobRejectsUnsafeRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/auth" {
			w.Write([]byte(`{"token":"tok"}`))
			return
		}
		http.Redirect(w, r, "http://example.invalid/blob", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	reg, err := New(srv.Client(), srv.URL).Registry(context.Background(), "toolbox")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Blob(context.Background(), "sha256:"+strings.Repeat("a", 64)); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("err = %v", err)
	}
}

func TestBlobDigestMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/auth" {
			w.Write([]byte(`{"token":"tok"}`))
			return
		}
		w.Write([]byte("tampered"))
	}))
	defer srv.Close()
	reg, _ := New(srv.Client(), srv.URL).Registry(context.Background(), "toolbox")
	if _, err := reg.Blob(context.Background(), "sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}
