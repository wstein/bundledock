package toolpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CIRun describes a GitHub Actions workflow run.
type CIRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

type runsResponse struct {
	TotalCount   int     `json:"total_count"`
	WorkflowRuns []CIRun `json:"workflow_runs"`
}

// GitHubPublisher queries GitHub Actions and dispatches publication workflows.
type GitHubPublisher struct {
	Client  *http.Client
	BaseURL string
	Repo    string
	Token   string
}

// ResolveToken returns the active GitHub token from explicit flag, environment,
// or the local gh CLI credentials.
func ResolveToken(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	for _, envVar := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if val := os.Getenv(envVar); val != "" {
			return val, nil
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		tok := strings.TrimSpace(string(out))
		if tok != "" {
			return tok, nil
		}
	}
	return "", fmt.Errorf("GitHub token required; provide --token, set GITHUB_TOKEN or authenticate via 'gh auth login'")
}

func (p *GitHubPublisher) baseURL() string {
	if p.BaseURL != "" {
		return strings.TrimSuffix(p.BaseURL, "/")
	}
	return "https://api.github.com"
}

func (p *GitHubPublisher) httpClient() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// VerifyCommitCI queries GitHub Actions to confirm the official CI workflow succeeded
// for the exact commit SHA on the default branch.
func (p *GitHubPublisher) VerifyCommitCI(ctx context.Context, sha string) (*CIRun, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/actions/runs?head_sha=%s&event=push&branch=main", p.baseURL(), p.Repo, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying CI runs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data runsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding CI runs response: %w", err)
	}

	for _, run := range data.WorkflowRuns {
		if run.Name == "CI" || strings.Contains(run.Name, "CI") {
			if run.Status != "completed" {
				return nil, fmt.Errorf("CI run is still in progress (status: %s); url: %s", run.Status, run.HTMLURL)
			}
			if run.Conclusion != "success" {
				return nil, fmt.Errorf("CI run completed with conclusion %q; url: %s", run.Conclusion, run.HTMLURL)
			}
			return &run, nil
		}
	}

	return nil, fmt.Errorf("no completed successful CI run found for commit %s on main", sha)
}

// DispatchPublication triggers the publish-bundles.yml workflow via workflow_dispatch.
func (p *GitHubPublisher) DispatchPublication(ctx context.Context, bundle, sha string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/actions/workflows/publish-bundles.yml/dispatches", p.baseURL(), p.Repo)
	payload := map[string]any{
		"ref": "main",
		"inputs": map[string]string{
			"bundle":       bundle,
			"approved_sha": sha,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("dispatching publication workflow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("workflow dispatch failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}
