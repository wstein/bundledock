package toolpublish

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type workflowRun struct {
	ID         int64  `json:"id"`
	Number     int64  `json:"run_number"`
	Attempt    int64  `json:"run_attempt"`
	WorkflowID int64  `json:"workflow_id"`
	Event      string `json:"event"`
	Branch     string `json:"head_branch"`
	Commit     string `json:"head_sha"`
	Created    string `json:"created_at"`
}

// CheckNewest refuses to promote an older run or attempt. Concurrency serializes
// promotions, but GitHub's pending queue has no ordering guarantee. A newer run
// supersedes an older one when it is eligible, even after failure. Proven
// resolved no-ops are ignored; unresolved eligibility fails closed. Start a fresh manual run
// to retry publication, rather than allowing an old source revision to roll back
// stable. The authenticated client never follows redirects off api.github.com.
func CheckNewest(ctx context.Context, client *http.Client, repository, branch string, id, attempt int64, commit, token string) error {
	return checkNewest(ctx, client, repository, branch, id, attempt, commit, token, "")
}

// CheckNewestForWorkflow also binds the run to its selected publication workflow.
func CheckNewestForWorkflow(ctx context.Context, client *http.Client, repository, branch string, id, attempt int64, commit, token, workflow string) error {
	if workflow != "bundledock-toolbox.yml" && workflow != "bundledock-code-server.yml" && workflow != "bundledock-node.yml" && workflow != "bundledock-bun.yml" && workflow != "bundledock-codex.yml" && workflow != "bundledock-agy.yml" && workflow != "bundledock-claude.yml" {
		return fmt.Errorf("invalid publication workflow")
	}
	return checkNewest(ctx, client, repository, branch, id, attempt, commit, token, workflow)
}
func checkNewest(ctx context.Context, client *http.Client, repository, branch string, id, attempt int64, commit, token, workflow string) error {
	if id < 1 || attempt < 1 || branch == "" || !commitPattern.MatchString(commit) || token == "" {
		return fmt.Errorf("complete GitHub run identity and token required")
	}
	github, err := newGithubReader(client, repository, token)
	if err != nil {
		return err
	}
	get := func(path string, out any) error { return github.get(ctx, path, out) }
	var current workflowRun
	if err := get("/actions/runs/"+strconv.FormatInt(id, 10), &current); err != nil {
		return err
	}
	if current.ID != id || current.Attempt != attempt || current.Commit != commit || current.Branch != branch || current.WorkflowID < 1 || current.Number < 1 || (current.Event != "schedule" && current.Event != "workflow_dispatch" && !(workflow != "" && current.Event == "workflow_run")) {
		return fmt.Errorf("current run identity is no longer eligible for publication")
	}
	if workflow != "" {
		var expected struct {
			ID int64 `json:"id"`
		}
		if err := get("/actions/workflows/"+workflow, &expected); err != nil {
			return err
		}
		if expected.ID < 1 || current.WorkflowID != expected.ID {
			return fmt.Errorf("run belongs to another publication workflow")
		}
	}
	if _, err := time.Parse(time.RFC3339, current.Created); err != nil {
		return fmt.Errorf("invalid GitHub run creation time")
	}
	found := false
	for page := 1; page <= 1000; page++ {
		query := url.Values{"branch": {branch}, "created": {">=" + current.Created}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		var list struct {
			Runs       []workflowRun `json:"workflow_runs"`
			TotalCount int           `json:"total_count"`
		}
		path := "/actions/workflows/" + strconv.FormatInt(current.WorkflowID, 10) + "/runs?" + query.Encode()
		if err := get(path, &list); err != nil {
			return err
		}
		if list.TotalCount > 1000 {
			return fmt.Errorf("GitHub run search exceeds 1000-result limit; start a fresh manual run")
		}
		for _, run := range list.Runs {
			if run.Branch != branch || (run.Event != "schedule" && run.Event != "workflow_dispatch" && !(workflow != "" && run.Event == "workflow_run")) {
				continue
			}
			if run.Number > current.Number {
				eligible, err := nativeEligibility(get, run)
				if err != nil {
					return err
				}
				if eligible {
					return fmt.Errorf("newer eligible workflow run %d supersedes this publication", run.ID)
				}
			}
			if run.ID == id {
				if run.Attempt != attempt {
					return fmt.Errorf("a newer attempt supersedes this publication")
				}
				found = true
			}
		}
		if len(list.Runs) < 100 {
			if !found {
				return fmt.Errorf("current run missing from GitHub publication eligibility response")
			}
			return nil
		}
	}
	return fmt.Errorf("cannot establish newest publication within GitHub pagination limit")
}

// A skipped native job alone is ambiguous: a failed resolver also skips it.
// Only successful resolution plus completed skipped native jobs proves a no-op.
func nativeEligibility(get func(string, any) error, run workflowRun) (bool, error) {
	if run.ID < 1 || run.Attempt < 1 {
		return false, fmt.Errorf("newer run lacks complete attempt identity")
	}
	resolved, native, eligible := false, false, false
	for page := 1; page <= 10; page++ {
		var list struct {
			Jobs []struct{ Name, Status, Conclusion string }
		}
		path := "/actions/runs/" + strconv.FormatInt(run.ID, 10) + "/attempts/" + strconv.FormatInt(run.Attempt, 10) + "/jobs?per_page=100&page=" + strconv.Itoa(page)
		if err := get(path, &list); err != nil {
			return false, err
		}
		for _, job := range list.Jobs {
			// Reusable workflow job names retain the real caller's prefix.
			parts := strings.Split(job.Name, " / ")
			job.Name = parts[len(parts)-1]
			if job.Name == "resolve" {
				resolved = job.Status == "completed" && job.Conclusion == "success"
			}
			if job.Name == "native" || strings.HasPrefix(job.Name, "native (") {
				native = true
				if job.Status != "completed" || job.Conclusion != "skipped" {
					eligible = true
				}
			}
		}
		if len(list.Jobs) < 100 {
			// A workflow_run is only a request until human approval and successful
			// resolution reach native work. Unrelated CI completions can queue behind
			// this workflow's concurrency group without superseding publication.
			// Current-main provenance is checked separately immediately before promotion.
			if run.Event == "workflow_run" {
				return resolved && eligible, nil
			}
			if eligible {
				return true, nil
			}
			if resolved && native {
				return false, nil
			}
			return false, fmt.Errorf("cannot establish newer run %d publication eligibility", run.ID)
		}
	}
	return false, fmt.Errorf("newer run jobs exceed pagination limit")
}
