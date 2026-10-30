package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	var out, errOut bytes.Buffer
	cmd := newRootCommandWithClient(srv.Client())
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"quay"}, append(args, "--api-url", srv.URL, "--root", "../..")...))
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
