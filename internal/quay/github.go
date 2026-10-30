package quay

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// GHAPI fetches one GitHub API path below repos/<owner>/<repo>/.
type GHAPI func(ctx context.Context, path string) ([]byte, error)

// GHCLI runs the gh command line tool, which supplies its own credentials.
func GHCLI(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "api", "repos/"+GitHubRepository+"/"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("GitHub evidence unavailable")
	}
	return out, nil
}

// GitHubAPI implements GitHub on top of GHAPI, with a per-path cache.
type GitHubAPI struct {
	Fetch GHAPI
	cache map[string][]byte
}

func (g *GitHubAPI) get(ctx context.Context, path string, fresh bool, v any) error {
	raw, ok := g.cache[path]
	if fresh || !ok {
		var err error
		if raw, err = g.Fetch(ctx, path); err != nil {
			return err
		}
		if len(raw) > maxBody {
			return fmt.Errorf("GitHub response too large")
		}
		if g.cache == nil {
			g.cache = map[string][]byte{}
		}
		g.cache[path] = raw
	}
	return json.Unmarshal(raw, v)
}

func validRun(runID string, attempt int) error {
	return check(positivePattern.MatchString(runID) && attempt > 0, "Invalid run attempt")
}

// Run implements GitHub.
func (g *GitHubAPI) Run(ctx context.Context, runID string, fresh bool) (Run, error) {
	var run Run
	if err := check(positivePattern.MatchString(runID), "Invalid run ID"); err != nil {
		return run, err
	}
	err := g.get(ctx, "actions/runs/"+runID, fresh, &run)
	return run, err
}

// Attempt implements GitHub.
func (g *GitHubAPI) Attempt(ctx context.Context, runID string, attempt int, fresh bool) (Run, error) {
	var run Run
	if err := validRun(runID, attempt); err != nil {
		return run, err
	}
	err := g.get(ctx, "actions/runs/"+runID+"/attempts/"+strconv.Itoa(attempt), fresh, &run)
	return run, err
}

// Workflow implements GitHub.
func (g *GitHubAPI) Workflow(ctx context.Context, fresh bool) (Workflow, error) {
	var w Workflow
	err := g.get(ctx, "actions/workflows/publish-bundles.yml", fresh, &w)
	return w, err
}

// Jobs implements GitHub and returns every job of the attempt, or an error.
func (g *GitHubAPI) Jobs(ctx context.Context, runID string, attempt int, fresh bool) ([]Job, error) {
	if err := validRun(runID, attempt); err != nil {
		return nil, err
	}
	var jobs []Job
	seen := map[int64]bool{}
	total := -1
	for page := 1; page <= 20; page++ {
		var data struct {
			TotalCount *int  `json:"total_count"`
			Jobs       []Job `json:"jobs"`
		}
		path := "actions/runs/" + runID + "/attempts/" + strconv.Itoa(attempt) + "/jobs?per_page=100&page=" + strconv.Itoa(page)
		if err := g.get(ctx, path, fresh, &data); err != nil {
			return nil, err
		}
		if err := check(data.TotalCount != nil && *data.TotalCount >= 0 && *data.TotalCount <= 2000 && data.Jobs != nil, "Malformed jobs pagination"); err != nil {
			return nil, err
		}
		if err := check(total < 0 || *data.TotalCount == total, "Jobs changed during pagination"); err != nil {
			return nil, err
		}
		total = *data.TotalCount
		for _, job := range data.Jobs {
			if err := check(!seen[job.ID], "Duplicate or invalid job"); err != nil {
				return nil, err
			}
			seen[job.ID] = true
			jobs = append(jobs, job)
		}
		if err := check(len(jobs) <= total, "Jobs exceed reported total"); err != nil {
			return nil, err
		}
		if len(jobs) == total {
			return jobs, nil
		}
		if err := check(len(data.Jobs) > 0, "Missing jobs page"); err != nil {
			return nil, err
		}
	}
	return nil, reasonError("Jobs pagination limit exceeded")
}
