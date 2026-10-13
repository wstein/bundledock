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
	for _, workflow := range []string{"publish-bundles.yml", "publish-bundles.yml", "publish-bundles.yml", "publish-bundles.yml", "publish-bundles.yml", "publish-bundles.yml", "publish-bundles.yml", "publish-bundles.yml"} {
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

func TestConsolidatedNewerPublicationRespectsRegistryScope(t *testing.T) {
	for _, tc := range []struct {
		name, marker string
		want         bool
	}{
		{"same bundle", "toolbox", true},
		{"other repository", "bun", false},
		{"shared node repository", "node-current", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := "toolbox"
			if tc.name == "shared node repository" {
				bundle = "node"
			}
			body := `{"jobs":[{"name":"publication / preflight (` + tc.marker + `)","status":"completed","conclusion":"success"},{"name":"publication / resolve","status":"completed","conclusion":"success"},{"name":"publication / native (amd64, ubuntu-24.04)","status":"in_progress"}]}`
			got, err := nativeEligibilityForBundle(func(_ string, out any) error { return json.Unmarshal([]byte(body), out) }, workflowRun{ID: 124, Attempt: 1, Event: "workflow_run"}, bundle)
			if err != nil || got != tc.want {
				t.Fatalf("eligible=%v error=%v", got, err)
			}
		})
	}
}

func TestConsolidatedBundleIdentityFailsClosed(t *testing.T) {
	for _, body := range []string{
		`{"jobs":[]}`,
		`{"jobs":[{"name":"publication / preflight (unknown)"}]}`,
		`{"jobs":[{"name":"publication / preflight (node)"},{"name":"publication / preflight (node)"}]}`,
		`{"jobs":[{"name":"plan (node)"},{"name":"publication / preflight (bun)"}]}`,
		`{"jobs":[{"name":"publication / preflight (node)"},{"name":"publication / preflight (bun)"}]}`,
	} {
		_, err := nativeEligibilityForBundle(func(_ string, out any) error { return json.Unmarshal([]byte(body), out) }, workflowRun{ID: 124, Attempt: 1, Event: "workflow_dispatch"}, "node")
		if err == nil {
			t.Fatal("accepted incomplete or ambiguous bundle identity")
		}
	}
	if err := CheckNewestForWorkflow(context.Background(), nil, "example/repo", "main", 123, 1, strings.Repeat("a", 40), "token", "bundledock-node.yml"); err == nil {
		t.Fatal("accepted retired caller for new publication")
	}
}

func TestConsolidatedManualPlannerDoesNotBlockOtherRegistries(t *testing.T) {
	for _, tc := range []struct {
		name, jobs string
		want       bool
		wantErr    bool
	}{
		{"rejected same registry", `{"jobs":[{"name":"plan (node)","status":"completed","conclusion":"failure"}]}`, false, false},
		{"rejected unrelated request", `{"jobs":[{"name":"plan (bun)","status":"completed","conclusion":"failure"}]}`, false, false},
		{"pending unrelated request", `{"jobs":[{"name":"plan (bun)","status":"in_progress"}]}`, false, false},
		{"same registry unresolved", `{"jobs":[{"name":"plan (node-current)","status":"completed","conclusion":"success"}]}`, false, true},
		{"matching planner and publisher", `{"jobs":[{"name":"plan (node)","status":"completed","conclusion":"success"},{"name":"publication / preflight (node)","status":"completed","conclusion":"success"},{"name":"publication / resolve","status":"completed","conclusion":"success"},{"name":"publication / native (amd64, ubuntu-24.04)","status":"in_progress"}]}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nativeEligibilityForBundle(func(_ string, out any) error { return json.Unmarshal([]byte(tc.jobs), out) }, workflowRun{ID: 124, Attempt: 1, Event: "workflow_dispatch"}, "node")
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("eligible=%v error=%v", got, err)
			}
		})
	}
}

func TestConsolidatedAutomaticMissingMarkerFailsClosedAfterPublicationStarts(t *testing.T) {
	for _, tc := range []struct {
		jobs    string
		wantErr bool
	}{
		{`{"jobs":[{"name":"plan (automatic)","status":"completed","conclusion":"success"}]}`, false},
		{`{"jobs":[{"name":"publication / resolve","status":"in_progress"}]}`, true},
		{`{"jobs":[{"name":"publication / resolve","status":"completed","conclusion":"success"},{"name":"publication / native (amd64, ubuntu-24.04)","status":"in_progress"}]}`, true},
	} {
		got, err := nativeEligibilityForBundle(func(_ string, out any) error { return json.Unmarshal([]byte(tc.jobs), out) }, workflowRun{ID: 124, Attempt: 1, Event: "workflow_run"}, "node")
		if got || (err != nil) != tc.wantErr {
			t.Fatalf("eligible=%v error=%v", got, err)
		}
	}
}
