package quay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeQuay serves the tag listing for the given repositories; others answer 404.
func fakeQuay(t *testing.T, repos map[string][]Tag) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/repository/bundledock/")
		repo, suffix, _ := strings.Cut(rest, "/")
		tags, found := repos[repo]
		if !ok || !found || suffix != "tag/" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"page": 1, "tags": append([]Tag{}, tags...), "has_additional": false})
	}))
}

// repoRoot is the repository root, which holds the committed publication records.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
