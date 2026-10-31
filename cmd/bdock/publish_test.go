package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublishHelp(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"publish", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"--all", "--yes", "--dry-run", "--repo", "--token"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help output missing %q: %s", want, text)
		}
	}
}

func TestPublishValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no args without all", []string{"publish"}, "requires a bundle name or --all"},
		{"bundle with all", []string{"publish", "agy", "--all"}, "cannot combine bundle argument and --all"},
		{"retired bundle", []string{"publish", "opentofu"}, "retired"},
		{"unknown bundle", []string{"publish", "unknown-bundle"}, "unknown canonical publication bundle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommand()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for %v, got nil", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestPublishGitPreflightErrors(t *testing.T) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, tc := range []struct {
		name    string
		clean   bool
		branch  string
		synced  bool
		wantErr string
	}{
		{"dirty tree", false, "main", true, "working tree has uncommitted changes"},
		{"wrong branch", true, "feat/something", true, "must be on branch main (currently on feat/something)"},
		{"unsynced main", true, "main", false, "local main differs from origin/main"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := gitPreflight
			t.Cleanup(func() { gitPreflight = original })
			gitPreflight = func() (string, string, bool, bool, error) {
				return sha, tc.branch, tc.clean, tc.synced, nil
			}

			var out bytes.Buffer
			cmd := newRootCommand()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"publish", "agy", "--token", "tok"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestPublishDryRun(t *testing.T) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	original := gitPreflight
	t.Cleanup(func() { gitPreflight = original })
	gitPreflight = func() (string, string, bool, bool, error) {
		return sha, "main", true, true, nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"workflow_runs": []map[string]any{
				{
					"id":          int64(99999),
					"name":        "CI",
					"head_sha":    sha,
					"head_branch": "main",
					"status":      "completed",
					"conclusion":  "success",
					"html_url":    "https://github.com/wstein/bundledock/actions/runs/99999",
				},
			},
		})
	}))
	defer server.Close()

	var out bytes.Buffer
	cmd := newRootCommandWithClient(server.Client())
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"publish", "agy", "--dry-run", "--token", "tok", "--server-url", server.URL})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	output := out.String()
	for _, want := range []string{"Preflight verification successful", "agy", sha, "[dry-run] Workflow dispatch skipped"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %s", want, output)
		}
	}
}

func TestPublishDispatchSuccess(t *testing.T) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	original := gitPreflight
	t.Cleanup(func() { gitPreflight = original })
	gitPreflight = func() (string, string, bool, bool, error) {
		return sha, "main", true, true, nil
	}

	var dispatchedBundle string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{
				"total_count": 1,
				"workflow_runs": []map[string]any{
					{
						"id":          int64(99999),
						"name":        "CI",
						"head_sha":    sha,
						"head_branch": "main",
						"status":      "completed",
						"conclusion":  "success",
						"html_url":    "https://github.com/wstein/bundledock/actions/runs/99999",
					},
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			inputs := body["inputs"].(map[string]any)
			dispatchedBundle = inputs["bundle"].(string)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	cmd := newRootCommandWithClient(server.Client())
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"publish", "agy", "--yes", "--token", "tok", "--server-url", server.URL})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if dispatchedBundle != "agy" {
		t.Fatalf("dispatched bundle = %q, want agy", dispatchedBundle)
	}
	if !strings.Contains(out.String(), "Dispatched publication for agy") {
		t.Fatalf("output missing confirmation: %s", out.String())
	}
}
