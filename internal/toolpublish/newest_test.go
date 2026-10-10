package toolpublish

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewestPublicationGate(t *testing.T) {
	current := `{"id":123,"run_number":10,"run_attempt":2,"event":"workflow_dispatch","head_branch":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-10-10T03:17:00Z","workflow_id":77}`
	for _, test := range []struct {
		name, list string
		wantErr    bool
	}{
		{"current", `{"workflow_runs":[` + current + `]}`, false},
		{"newer queued run", `{"workflow_runs":[{"id":124,"run_number":11,"run_attempt":1,"event":"schedule","head_branch":"main"},` + current + `]}`, true},
		{"newer failed run", `{"workflow_runs":[` + current + `,{"id":124,"run_number":11,"run_attempt":1,"event":"workflow_dispatch","head_branch":"main","conclusion":"failure"}]}`, true},
		{"current missing", `{"workflow_runs":[]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer private-token" {
					t.Fatal("unexpected credential destination")
				}
				body := current
				if calls == 2 {
					body = test.list
					if r.URL.Path != "/repos/example/repo/actions/workflows/77/runs" || r.URL.Query().Get("branch") != "main" || r.URL.Query().Get("created") != ">=2026-10-10T03:17:00Z" {
						t.Fatalf("unbounded or wrong workflow query %s", r.URL)
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			err := CheckNewest(context.Background(), client, "example/repo", "main", 123, 2, strings.Repeat("a", 40), "private-token")
			if (err != nil) != test.wantErr {
				t.Fatalf("got %v, want error %v", err, test.wantErr)
			}
		})
	}
}

func TestNewestPublicationFailsClosed(t *testing.T) {
	for _, scenario := range []string{"http", "attempt", "commit", "branch", "event", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			current := map[string]any{"id": 123, "run_number": 10, "run_attempt": 2, "event": "workflow_dispatch", "head_branch": "main", "head_sha": strings.Repeat("a", 40), "created_at": "2026-10-10T03:17:00Z", "workflow_id": 77}
			switch scenario {
			case "attempt":
				current["run_attempt"] = 3
			case "commit":
				current["head_sha"] = strings.Repeat("b", 40)
			case "branch":
				current["head_branch"] = "feature"
			case "event":
				current["event"] = "pull_request"
			}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				status := 200
				headers := make(http.Header)
				if scenario == "http" {
					status = 503
				}
				if scenario == "redirect" {
					status = 302
					headers.Set("Location", "https://untrusted.example/collect")
				}
				if r.URL.Host != "api.github.com" {
					t.Fatal("followed authenticated redirect")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"id":%v,"run_number":%v,"run_attempt":%v,"event":%q,"head_branch":%q,"head_sha":%q,"created_at":%q,"workflow_id":%v}`, current["id"], current["run_number"], current["run_attempt"], current["event"], current["head_branch"], current["head_sha"], current["created_at"], current["workflow_id"]))), Header: headers}, nil
			})}
			err := CheckNewest(context.Background(), client, "example/repo", "main", 123, 2, strings.Repeat("a", 40), "private-token")
			if err == nil || strings.Contains(err.Error(), "private-token") {
				t.Fatalf("gate did not fail safely: %v", err)
			}
		})
	}
}
