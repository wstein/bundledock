package toolpublish

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMissingStableRequiresAuthenticatedExact404(t *testing.T) {
	for _, tc := range []struct {
		name         string
		auth, status int
		body         string
		absent       bool
	}{
		{"missing stable", 200, 404, "", true},
		{"auth401", 401, 404, "", false}, {"auth403", 403, 404, "", false},
		{"denied stable", 200, 403, "", false}, {"registry failure", 200, 503, "", false},
		{"redirect", 200, 302, "", false}, {"malformed", 200, 200, "invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "quay.io" {
					t.Fatal("wrong registry host")
				}
				code, body := tc.status, tc.body
				if r.URL.Path == "/v2/auth" {
					code = tc.auth
					body = `{"token":"pull-only-fixture"}`
					if r.URL.Query().Get("scope") != "repository:example/code-server:pull" {
						t.Fatal("wrong namespace/scope")
					}
				} else {
					if r.URL.Path != "/v2/example/code-server/manifests/stable" || r.Header.Get("Authorization") != "Bearer pull-only-fixture" {
						t.Fatal("wrong stable request")
					}
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Location": []string{"https://untrusted.invalid/"}}}, nil
			})}
			_, err := PublishedToolDigest(context.Background(), client, "quay.io/example/code-server", "robot", "fixture-token", nil)
			if err == nil || errors.Is(err, ErrBaselineAbsent) != tc.absent {
				t.Fatalf("error %v absent=%v", err, tc.absent)
			}
		})
	}
}
func TestCodeServerNewestBindsDedicatedWorkflow(t *testing.T) {
	current := `{"id":123,"run_number":10,"run_attempt":2,"event":"workflow_dispatch","head_branch":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-10-10T03:17:00Z","workflow_id":77}`
	for _, workflowID := range []string{"77", "78"} {
		t.Run(workflowID, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := current
				switch r.URL.Path {
				case "/repos/example/repo/actions/workflows/bundledock-code-server.yml":
					body = `{"id":` + workflowID + `}`
				case "/repos/example/repo/actions/workflows/77/runs":
					body = `{"workflow_runs":[` + current + `]}`
				case "/repos/example/repo/actions/runs/123":
				default:
					t.Fatal("unexpected workflow request " + r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			err := CheckNewestForWorkflow(context.Background(), client, "example/repo", "main", 123, 2, strings.Repeat("a", 40), "fixture-token", "bundledock-code-server.yml")
			if (err != nil) != (workflowID != "77") {
				t.Fatalf("identity %s: %v", workflowID, err)
			}
		})
	}
}
