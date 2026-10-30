package quay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

type rewriteTransport struct{ target *url.URL }

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host, req.Host = r.target.Scheme, r.target.Host, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// clientFor returns a client for the default quay.io URL whose requests go to
// srv, so code that sends credentials (only allowed to quay.io) can be tested.
func clientFor(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return New(&http.Client{Transport: rewriteTransport{target}}, "")
}
