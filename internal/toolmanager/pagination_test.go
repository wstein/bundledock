package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestGitHubPaginationKeepsConfiguredRepository(t *testing.T) {
	c, err := toolconfig.Load(checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	tag := c.Catalog.Packages["ast-grep"].TagPrefix + c.Desired["ast-grep"].Lower.String()
	const endpoint = "https://api.github.com/repos/ast-grep/ast-grep/releases"
	for _, tc := range []struct {
		name, next string
		valid      bool
	}{
		{"named", endpoint + "?per_page=100&page=2", true},
		// Observed from the public ast-grep releases API on 2026-10-10.
		{"actual-numeric-alias", "https://api.github.com/repositories/509566212/releases?per_page=100&page=2", true},
		// Numeric IDs never become request targets: retain the configured repo.
		{"numeric-alias-cannot-switch-repo", "https://api.github.com/repositories/123/releases?per_page=100&page=2", true},
		{"foreign-host", "https://evil.example/repositories/509566212/releases?per_page=100&page=2", false},
		{"insecure", strings.Replace(endpoint, "https:", "http:", 1) + "?per_page=100&page=2", false},
		{"userinfo", strings.Replace(endpoint, "https://", "https://user@", 1) + "?per_page=100&page=2", false},
		{"port", strings.Replace(endpoint, "api.github.com/", "api.github.com:443/", 1) + "?per_page=100&page=2", false},
		{"other-named-repo", "https://api.github.com/repos/other/repo/releases?per_page=100&page=2", false},
		{"other-resource", "https://api.github.com/repositories/509566212/issues?per_page=100&page=2", false},
		{"invalid-id", "https://api.github.com/repositories/not-an-id/releases?per_page=100&page=2", false},
		{"encoded-path", "https://api.github.com/repos/ast-grep/%61st-grep/releases?per_page=100&page=2", false},
		{"fragment", endpoint + "?per_page=100&page=2#other", false},
		{"extra-query", endpoint + "?per_page=100&page=2&other=1", false},
		{"duplicate-page", endpoint + "?per_page=100&page=2&page=3", false},
		{"invalid-page", endpoint + "?per_page=100&page=abc", false},
		{"zero-page", endpoint + "?per_page=100&page=0", false},
		{"negative-page", endpoint + "?per_page=100&page=-2", false},
		{"missing-page", endpoint + "?per_page=100", false},
		{"changed-page-size", endpoint + "?per_page=1&page=2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", "dummy-pagination-token")
			calls := 0
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/repos/ast-grep/ast-grep/releases" || r.Header.Get("Authorization") != "Bearer dummy-pagination-token" {
					t.Errorf("pagination left configured repository or authentication boundary: %s", r.URL.Path)
				}
				if calls == 1 {
					w.Header().Set("Link", "<"+tc.next+">; rel=\"next\"")
					fmt.Fprint(w, "[]")
					return
				}
				fmt.Fprintf(w, `[{"tag_name":%q}]`, tag)
			})
			rel, err := (resolver{client: client}).github(context.Background(), c.Catalog.Packages["ast-grep"], c.Desired["ast-grep"])
			if tc.valid {
				if err != nil || rel.Tag != tag || calls != 2 {
					t.Fatalf("valid pagination: tag %q, calls %d, error %v", rel.Tag, calls, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "unsafe GitHub pagination URL") || calls != 1 {
				t.Fatalf("unsafe pagination was not rejected before another request: calls %d, error %v", calls, err)
			}
		})
	}
}
