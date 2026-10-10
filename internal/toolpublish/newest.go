package toolpublish

import (
	"context"
	"encoding/json"
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
// supersedes an older one even when the newer run fails: start a fresh manual run
// to retry publication, rather than allowing an old source revision to roll back
// stable. The authenticated client never follows redirects off api.github.com.
func CheckNewest(ctx context.Context, client *http.Client, repository, branch string, id, attempt int64, commit, token string) error {
	if id < 1 || attempt < 1 || branch == "" || !commitPattern.MatchString(commit) || token == "" {
		return fmt.Errorf("complete GitHub run identity and token required")
	}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repository, "?%#\\") {
		return fmt.Errorf("invalid GitHub repository")
	}
	guarded := http.Client{Timeout: 30 * time.Second}
	if client != nil {
		guarded = *client
	}
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	get := func(path string, out any) error {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+repository+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		response, err := guarded.Do(req)
		if err != nil {
			return fmt.Errorf("GitHub newest-run query failed: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub newest-run query returned HTTP %d", response.StatusCode)
		}
		if err = json.NewDecoder(response.Body).Decode(out); err != nil {
			return fmt.Errorf("invalid GitHub run response: %w", err)
		}
		return nil
	}
	var current workflowRun
	if err := get("/actions/runs/"+strconv.FormatInt(id, 10), &current); err != nil {
		return err
	}
	if current.ID != id || current.Attempt != attempt || current.Commit != commit || current.Branch != branch || current.WorkflowID < 1 || current.Number < 1 || (current.Event != "schedule" && current.Event != "workflow_dispatch") {
		return fmt.Errorf("current run identity is no longer eligible for publication")
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
			if run.Branch != branch || (run.Event != "schedule" && run.Event != "workflow_dispatch") {
				continue
			}
			if run.Number > current.Number {
				return fmt.Errorf("newer eligible workflow run %d supersedes this publication", run.ID)
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
