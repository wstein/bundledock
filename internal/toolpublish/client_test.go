package toolpublish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyCommitCI(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runs    []map[string]any
		wantErr string
		wantID  int64
		wantURL string
	}{
		{
			name: "successful CI",
			runs: []map[string]any{
				{
					"id":          int64(12345),
					"name":        "CI",
					"head_sha":    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"head_branch": "main",
					"status":      "completed",
					"conclusion":  "success",
					"html_url":    "https://github.com/wstein/bundledock/actions/runs/12345",
				},
			},
			wantID:  12345,
			wantURL: "https://github.com/wstein/bundledock/actions/runs/12345",
		},
		{
			name: "failed CI",
			runs: []map[string]any{
				{
					"id":          int64(12346),
					"name":        "CI",
					"head_sha":    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"head_branch": "main",
					"status":      "completed",
					"conclusion":  "failure",
					"html_url":    "https://github.com/wstein/bundledock/actions/runs/12346",
				},
			},
			wantErr: "CI run completed with conclusion \"failure\"",
		},
		{
			name: "in progress CI",
			runs: []map[string]any{
				{
					"id":          int64(12347),
					"name":        "CI",
					"head_sha":    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"head_branch": "main",
					"status":      "in_progress",
					"html_url":    "https://github.com/wstein/bundledock/actions/runs/12347",
				},
			},
			wantErr: "CI run is still in progress (status: in_progress)",
		},
		{
			name:    "no runs found",
			runs:    []map[string]any{},
			wantErr: "no completed successful CI run found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{
					"total_count":   len(tc.runs),
					"workflow_runs": tc.runs,
				})
			}))
			defer server.Close()

			pub := &GitHubPublisher{
				Client:  server.Client(),
				BaseURL: server.URL,
				Repo:    "wstein/bundledock",
				Token:   "test-token",
			}

			run, err := pub.VerifyCommitCI(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if run.ID != tc.wantID || run.HTMLURL != tc.wantURL {
				t.Fatalf("got %+v, want ID=%d URL=%q", run, tc.wantID, tc.wantURL)
			}
		})
	}
}

func TestDispatchPublication(t *testing.T) {
	var receivedBody map[string]any
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		receivedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	pub := &GitHubPublisher{
		Client:  server.Client(),
		BaseURL: server.URL,
		Repo:    "wstein/bundledock",
		Token:   "test-token",
	}

	err := pub.DispatchPublication(context.Background(), "agy", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}

	expectedPath := "/repos/wstein/bundledock/actions/workflows/publish-bundles.yml/dispatches"
	if receivedPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, receivedPath)
	}
	if receivedBody["ref"] != "main" {
		t.Fatalf("expected ref main, got %v", receivedBody["ref"])
	}
	inputs, ok := receivedBody["inputs"].(map[string]any)
	if !ok || inputs["bundle"] != "agy" || inputs["approved_sha"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected inputs: %v", inputs)
	}
}
