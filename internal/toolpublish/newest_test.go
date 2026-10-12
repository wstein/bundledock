package toolpublish

import (
	"context"
	"encoding/json"
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

func TestNewerNoopDoesNotSupersedePublication(t *testing.T) {
	current := `{"id":123,"run_number":10,"run_attempt":2,"event":"workflow_dispatch","head_branch":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-10-10T03:17:00Z","workflow_id":77}`
	for _, test := range []struct {
		name, jobs string
		wantErr    bool
	}{
		{"resolved no-op", `{"jobs":[{"name":"resolve","status":"completed","conclusion":"success"},{"name":"native","status":"completed","conclusion":"skipped"}]}`, false},
		{"changed both architectures", `{"jobs":[{"name":"resolve","status":"completed","conclusion":"success"},{"name":"native (amd64, ubuntu-24.04)","status":"completed","conclusion":"success"},{"name":"native (arm64, ubuntu-24.04-arm)","status":"completed","conclusion":"failure"}]}`, true},
		{"resolver failure", `{"jobs":[{"name":"resolve","status":"completed","conclusion":"failure"},{"name":"native","status":"completed","conclusion":"skipped"}]}`, true},
		{"resolver pending", `{"jobs":[{"name":"resolve","status":"in_progress"}]}`, true},
		{"missing eligibility", `{"jobs":[]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			jobCalls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := current
				if strings.Contains(r.URL.Path, "/workflows/") {
					body = `{"workflow_runs":[{"id":124,"run_number":11,"run_attempt":1,"event":"schedule","head_branch":"main"},` + current + `]}`
				}
				if strings.Contains(r.URL.Path, "/jobs") {
					jobCalls++
					if r.URL.Path != "/repos/example/repo/actions/runs/124/attempts/1/jobs" {
						t.Fatalf("wrong job attempt: %s", r.URL)
					}
					body = test.jobs
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			err := CheckNewest(context.Background(), client, "example/repo", "main", 123, 2, strings.Repeat("a", 40), "private-token")
			if (err != nil) != test.wantErr || jobCalls != 1 {
				t.Fatalf("got %v with %d job queries, want error=%t", err, jobCalls, test.wantErr)
			}
		})
	}
}

func TestNewerNoopJobPaginationIsBounded(t *testing.T) {
	calls := 0
	eligible, err := nativeEligibility(func(path string, out any) error {
		calls++
		if !strings.Contains(path, "/actions/runs/124/attempts/3/jobs?") {
			t.Fatalf("unbound attempt query %s", path)
		}
		body := `{"jobs":[{"name":"resolve","status":"completed","conclusion":"success"}`
		for i := 1; i < 100; i++ {
			body += `,{"name":"unrelated","status":"completed","conclusion":"success"}`
		}
		body += `]}`
		return json.Unmarshal([]byte(body), out)
	}, workflowRun{ID: 124, Attempt: 3})
	if err == nil || eligible || calls != 10 {
		t.Fatalf("unbounded eligibility search: %t %v %d", eligible, err, calls)
	}
}

func TestNewestWorkflowRunBindsEachBundleCaller(t *testing.T) {
	current := `{"id":123,"run_number":10,"run_attempt":2,"event":"workflow_run","head_branch":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-10-10T03:17:00Z","workflow_id":77}`
	for _, workflow := range []string{"bundledock-toolbox.yml", "bundledock-node.yml", "bundledock-bun.yml", "bundledock-code-server.yml", "bundledock-codex.yml", "bundledock-agy.yml", "bundledock-claude.yml"} {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := current
			if strings.Contains(r.URL.Path, "/workflows/") {
				body = `{"id":77}`
			}
			if strings.HasSuffix(r.URL.Path, "/runs") {
				body = `{"workflow_runs":[` + current + `]}`
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
		err := CheckNewestForWorkflow(context.Background(), client, "example/repo", "main", 123, 2, strings.Repeat("a", 40), "token", workflow)
		if err != nil {
			t.Fatalf("workflow=%s error=%v", workflow, err)
		}
	}
}

func TestNewerAutomaticRunSupersedesOnlyAfterApprovalAndResolution(t *testing.T) {
	for _, tc := range []struct {
		name, jobs string
		eligible   bool
	}{
		{"queued unrelated CI", `{"jobs":[]}`, false},
		{"rejected preflight", `{"jobs":[{"name":"preflight","status":"completed","conclusion":"failure"},{"name":"native","status":"completed","conclusion":"skipped"}]}`, false},
		{"waiting approval", `{"jobs":[{"name":"preflight","status":"completed","conclusion":"success"},{"name":"Approve toolbox SHA","status":"waiting"}]}`, false},
		{"failed resolver", `{"jobs":[{"name":"resolve","status":"completed","conclusion":"failure"},{"name":"native","status":"completed","conclusion":"skipped"}]}`, false},
		{"approved native", `{"jobs":[{"name":"resolve","status":"completed","conclusion":"success"},{"name":"native (amd64, ubuntu-24.04)","status":"in_progress"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eligible, err := nativeEligibility(func(_ string, out any) error { return json.Unmarshal([]byte(tc.jobs), out) }, workflowRun{ID: 124, Attempt: 1, Event: "workflow_run"})
			if err != nil || eligible != tc.eligible {
				t.Fatalf("eligible=%v error=%v", eligible, err)
			}
		})
	}
}
