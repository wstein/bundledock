package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type quayTag struct {
	Name           string `json:"name"`
	ManifestDigest string `json:"manifest_digest"`
	LastModified   string `json:"last_modified"`
}

// fakeQuayAPI serves the tag listing of the given repositories (empty for the
// other bundle repositories) so tests never reach the network.
func fakeQuayAPI(t *testing.T, repos map[string][]quayTag) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, _ := strings.CutPrefix(r.URL.Path, "/api/v1/repository/bundledock/")
		repo, suffix, _ := strings.Cut(rest, "/")
		if suffix != "tag/" {
			http.NotFound(w, r)
			return
		}
		tags := repos[repo]
		if tags == nil {
			tags = []quayTag{}
		}
		json.NewEncoder(w).Encode(map[string]any{"page": 1, "tags": tags, "has_additional": false})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runQuay(t *testing.T, srv *httptest.Server, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runQuayAt(t, "../..", srv, args...)
}

func runQuayAt(t *testing.T, root string, srv *httptest.Server, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := newRootCommandWithClient(srv.Client())
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"quay"}, append(args, "--api-url", srv.URL, "--root", root)...))
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestQuayList(t *testing.T) {
	srv := fakeQuayAPI(t, map[string][]quayTag{"toolbox": {
		{Name: "2026.10.30-b3", ManifestDigest: "sha256:aa", LastModified: "Fri, 30 Oct 2026 10:00:00 -0000"},
		{Name: "staging-2026.10.30-b4-amd64", ManifestDigest: "sha256:bb"},
		{Name: "staging-2026.10.30-b4-arm64", ManifestDigest: "sha256:cc"},
	}})
	out, _, err := runQuay(t, srv, "list")
	if err != nil || !strings.Contains(out, "toolbox") || !strings.Contains(out, "2026.10.30-b3") || !strings.Contains(out, "sha256:aa") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	out, _, err = runQuay(t, srv, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Schema  string `json:"schema"`
		Bundles []struct {
			Bundle  string `json:"bundle"`
			Version string `json:"version"`
			Digest  string `json:"digest"`
			Staging int    `json:"staging_tags"`
		} `json:"bundles"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Schema != "bdock.quay.list/v1" || len(doc.Bundles) != 10 {
		t.Fatalf("doc = %+v, err = %v", doc, err)
	}
	for _, b := range doc.Bundles {
		if b.Bundle == "toolbox" && (b.Version != "2026.10.30-b3" || b.Staging != 2) {
			t.Fatalf("toolbox = %+v", b)
		}
	}
}

// writeRecord writes a publication record below a temporary root.
func writeRecord(t *testing.T, root, bundle, tag, digest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	record := `{"bundle":"` + bundle + `","image":"quay.io/bundledock/` + bundle + `","tag":"` + tag + `","digest":"` + digest + `"}`
	if err := os.WriteFile(filepath.Join(root, "docs", bundle+"-published.json"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQuayStatus(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	root := t.TempDir()
	writeRecord(t, root, "toolbox", "2026.10.30-b3", digest)
	good := []quayTag{{Name: "2026.10.30-b3", ManifestDigest: digest}, {Name: "latest", ManifestDigest: digest}, {Name: "stable", ManifestDigest: digest}}
	srv := fakeQuayAPI(t, map[string][]quayTag{"toolbox": good})
	out, _, err := runQuayAt(t, root, srv, "status")
	if err != nil || !strings.Contains(out, "toolbox") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	// A moved alias is a difference, with and without --json.
	bad := append([]quayTag{}, good[:2]...)
	bad = append(bad, quayTag{Name: "stable", ManifestDigest: "sha256:" + strings.Repeat("b", 64)})
	srv = fakeQuayAPI(t, map[string][]quayTag{"toolbox": bad})
	for _, args := range [][]string{{"status"}, {"status", "--json"}, {"status", "toolbox"}} {
		out, _, err = runQuayAt(t, root, srv, args...)
		if err == nil {
			t.Fatalf("%v: expected exit 1, out = %q", args, out)
		}
	}
	out, _, _ = runQuayAt(t, root, srv, "status", "--json")
	var doc struct {
		Schema  string `json:"schema"`
		Bundles []struct {
			Bundle, State string
		} `json:"bundles"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Schema != "bdock.quay.status/v1" || len(doc.Bundles) != 10 || doc.Bundles[0].State != "differs" || doc.Bundles[1].State != "skipped" {
		t.Fatalf("doc = %+v, err = %v, out = %s", doc, err, out)
	}
	if _, _, err := runQuayAt(t, root, srv, "status", "nope"); err == nil {
		t.Fatal("unknown bundle accepted")
	}
}

func TestQuayInfo(t *testing.T) {
	config := []byte(`{"os":"linux","architecture":"amd64","config":{"Labels":{"org.opencontainers.image.version":"1.2.3-b1","org.opencontainers.image.revision":"deadbeef"}}}`)
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(config))
	manifest := []byte(`{"config":{"digest":"` + configDigest + `"}}`)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
	blobs := map[string][]byte{configDigest: config, digest: manifest}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/auth":
			w.Write([]byte(`{"token":"tok"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/bundledock/bun/"):
			parts := strings.Split(r.URL.Path, "/")
			if data, ok := blobs[parts[len(parts)-1]]; ok {
				w.Write(data)
				return
			}
			http.NotFound(w, r)
		case r.URL.Path == "/api/v1/repository/bundledock/bun/tag/":
			json.NewEncoder(w).Encode(map[string]any{"page": 1, "has_additional": false, "tags": []quayTag{{Name: "1.2.3-b1", ManifestDigest: digest, LastModified: "Fri, 30 Oct 2026 10:00:00 -0000"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	out, _, err := runQuay(t, srv, "info", "bun")
	if err != nil || !strings.Contains(out, "linux/amd64") || !strings.Contains(out, "deadbeef") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	out, _, err = runQuay(t, srv, "info", "bun", "--json")
	var doc struct {
		Schema string `json:"schema"`
		Tags   []struct {
			Tag, Version, Revision string
		} `json:"tags"`
	}
	if err != nil || json.Unmarshal([]byte(out), &doc) != nil || doc.Schema != "bdock.quay.info/v1" || len(doc.Tags) != 1 || doc.Tags[0].Version != "1.2.3-b1" {
		t.Fatalf("doc = %+v, err = %v", doc, err)
	}
	if _, _, err := runQuay(t, srv, "info"); err == nil {
		t.Fatal("bundle argument not required")
	}
}

func TestQuayDescribe(t *testing.T) {
	live := "old"
	token := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository/bundledock/toolbox" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPut {
			token = r.Header.Get("Authorization")
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			live = body["description"]
			w.Write([]byte("{}"))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"description": live, "is_public": true})
	}))
	defer srv.Close()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bundles", "toolbox"), 0o755)
	os.WriteFile(filepath.Join(root, "bundles", "toolbox", "description.md"), []byte("new\n"), 0o644)
	out, _, err := runQuayAt(t, root, srv, "describe", "--json")
	if err == nil || !strings.Contains(out, `"differs"`) || !strings.Contains(out, "bdock.quay.describe/v1") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	t.Setenv("QUAY_TOKEN", "")
	if _, _, err = runQuayAt(t, root, srv, "describe", "--apply"); err == nil {
		t.Fatal("apply without QUAY_TOKEN succeeded")
	}
	t.Setenv("QUAY_TOKEN", "secret-token")
	out, errOut, err := runQuayAt(t, root, srv, "describe", "--apply")
	if err != nil || live != "new" || token != "Bearer secret-token" || strings.Contains(out+errOut, "secret-token") {
		t.Fatalf("apply: err = %v live = %q token set = %v", err, live, token != "")
	}
	if _, _, err = runQuayAt(t, root, srv, "describe"); err != nil {
		t.Fatalf("second check: %v", err)
	}
	if _, _, err = runQuayAt(t, root, srv, "describe", "bun"); err == nil {
		t.Fatal("bundle without description file accepted")
	}
	if _, _, err = runQuayAt(t, root, srv, "describe", "--token", "x"); err == nil {
		t.Fatal("token flag exists")
	}
}

func TestCommittedDescriptions(t *testing.T) {
	for _, name := range []string{"toolbox", "nixbox"} {
		raw, err := os.ReadFile("../../bundles/" + name + "/description.md")
		if err != nil || len([]rune(string(raw))) > 5000 || !strings.Contains(string(raw), "https://wstein.github.io/bundledock/") || !strings.Contains(string(raw), "https://github.com/wstein/bundledock") {
			t.Fatalf("%s description: err = %v", name, err)
		}
	}
}
