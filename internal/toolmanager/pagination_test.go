package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestGitHubPaginationKeepsConfiguredRepository(t *testing.T) {
	// ast-grep is pinned in the nixbox tools directory.
	c, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	tag := c.Catalog.Packages["ast-grep"].TagPrefix + c.Desired["ast-grep"].Lower.String()
	const endpoint = "https://api.github.com/repos/ast-grep/ast-grep/releases"
	for _, tc := range []struct {
		name, next string
		valid      bool
	}{
		{"named", endpoint + "?per_page=20&page=2", true},
		// Observed from the public ast-grep releases API on 2026-10-10.
		{"actual-numeric-alias", "https://api.github.com/repositories/509566212/releases?per_page=20&page=2", true},
		// Numeric IDs never become request targets: retain the configured repo.
		{"numeric-alias-cannot-switch-repo", "https://api.github.com/repositories/123/releases?per_page=20&page=2", true},
		{"foreign-host", "https://evil.example/repositories/509566212/releases?per_page=20&page=2", false},
		{"insecure", strings.Replace(endpoint, "https:", "http:", 1) + "?per_page=20&page=2", false},
		{"userinfo", strings.Replace(endpoint, "https://", "https://user@", 1) + "?per_page=20&page=2", false},
		{"port", strings.Replace(endpoint, "api.github.com/", "api.github.com:443/", 1) + "?per_page=20&page=2", false},
		{"other-named-repo", "https://api.github.com/repos/other/repo/releases?per_page=20&page=2", false},
		{"other-resource", "https://api.github.com/repositories/509566212/issues?per_page=20&page=2", false},
		{"invalid-id", "https://api.github.com/repositories/not-an-id/releases?per_page=20&page=2", false},
		{"encoded-path", "https://api.github.com/repos/ast-grep/%61st-grep/releases?per_page=20&page=2", false},
		{"fragment", endpoint + "?per_page=20&page=2#other", false},
		{"extra-query", endpoint + "?per_page=20&page=2&other=1", false},
		{"duplicate-page", endpoint + "?per_page=20&page=2&page=3", false},
		{"invalid-page", endpoint + "?per_page=20&page=abc", false},
		{"zero-page", endpoint + "?per_page=20&page=0", false},
		{"negative-page", endpoint + "?per_page=20&page=-2", false},
		{"missing-page", endpoint + "?per_page=20", false},
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

func TestGitHubReleasePageSizeKeepsMetadataBounded(t *testing.T) {
	c, err := toolconfig.Load(checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	p, constraint := c.Catalog.Packages["codex"], c.Desired["codex"]
	tag := p.TagPrefix + constraint.Lower.String()
	requested := 0
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		size, parseErr := strconv.Atoi(r.URL.Query().Get("per_page"))
		requested = size
		if parseErr != nil || requested < 1 || requested > 100 {
			t.Errorf("invalid release page size %d: %v", requested, parseErr)
			http.Error(w, "invalid page size", http.StatusBadRequest)
			return
		}
		// Release bodies/assets can make a 100-release page exceed 16 MiB.
		page := make([]map[string]string, requested)
		for i := range page {
			page[i] = map[string]string{"tag_name": tag, "body": strings.Repeat("x", 170<<10)}
		}
		json.NewEncoder(w).Encode(page)
	})
	rel, err := (resolver{client: client}).github(context.Background(), p, constraint)
	if err != nil || rel.Tag != tag || requested != 20 {
		t.Fatalf("large release page: size %d, tag %q, error %v", requested, rel.Tag, err)
	}
}

func TestGitHubSmallerPagesRetainBoundedOlderPinDiscovery(t *testing.T) {
	c, err := toolconfig.Load(checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["codex"]
	constraint := toolconfig.Constraint{Lower: toolconfig.Version{0, 1, 0}, Exact: true}
	tag := p.TagPrefix + constraint.Lower.String()
	for _, exceedsCap := range []bool{false, true} {
		t.Run(fmt.Sprint(exceedsCap), func(t *testing.T) {
			calls := 0
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls < 100 || exceedsCap {
					w.Header().Set("Link", fmt.Sprintf("<https://api.github.com/repositories/965415649/releases?per_page=20&page=%d>; rel=\"next\"", calls+1))
				}
				if calls == 100 {
					fmt.Fprintf(w, `[{"tag_name":%q}]`, tag)
				} else {
					fmt.Fprint(w, "[]")
				}
			})
			rel, err := (resolver{client: client}).github(context.Background(), p, constraint)
			if calls != 100 {
				t.Fatalf("older release discovery stopped after %d pages: %v", calls, err)
			}
			if exceedsCap {
				if err == nil || !strings.Contains(err.Error(), "excessive GitHub pagination") {
					t.Fatalf("page cap did not fail closed: %v", err)
				}
			} else if err != nil || rel.Tag != tag {
				t.Fatalf("older exact pin was not discovered: tag %q, error %v", rel.Tag, err)
			}
		})
	}
}

func TestGitHubReleasePageExceeding8MiBBound(t *testing.T) {
	c, err := toolconfig.Load(checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	p, constraint := c.Catalog.Packages["codex"], c.Desired["codex"]
	tag := p.TagPrefix + constraint.Lower.String()
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		// A 20-release page exceeding the historical 8 MiB bound (e.g. 10 MiB)
		// must succeed within the 16 MiB bound.
		page := make([]map[string]string, 20)
		for i := range page {
			page[i] = map[string]string{"tag_name": tag, "body": strings.Repeat("x", 500<<10)}
		}
		json.NewEncoder(w).Encode(page)
	})
	rel, err := (resolver{client: client}).github(context.Background(), p, constraint)
	if err != nil || rel.Tag != tag {
		t.Fatalf("expected release %q to succeed, got error: %v", tag, err)
	}
}
