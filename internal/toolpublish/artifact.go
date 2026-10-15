package toolpublish

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type githubReader struct {
	client            *http.Client
	repository, token string
}

func newGithubReader(client *http.Client, repository, token string) (*githubReader, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repository, "?%#\\") || token == "" {
		return nil, fmt.Errorf("valid GitHub repository and token required")
	}
	guarded := http.Client{Timeout: 30 * time.Second}
	if client != nil {
		guarded = *client
	}
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &githubReader{&guarded, repository, token}, nil
}

func (g *githubReader) request(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+g.repository+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	response, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub publication query failed")
	}
	return response, nil
}

func (g *githubReader) get(ctx context.Context, path string, out any) error {
	r, err := g.request(ctx, path)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub publication query returned HTTP %d", r.StatusCode)
	}
	b, err := readBounded(r.Body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("invalid GitHub publication response")
	}
	return nil
}

// PublishedLockArtifact is only a migration fallback for images without a
// durable semantic label. Expiry/deletion cannot turn into a first publication.
func PublishedLockArtifact(ctx context.Context, client *http.Client, repository, branch, token string, m Metadata) ([]byte, error) {
	g, err := newGithubReader(client, repository, token)
	if err != nil {
		return nil, err
	}
	if m.Source != "https://github.com/"+repository || branch == "" || !runIDPattern.MatchString(m.RunID) {
		return nil, fmt.Errorf("published source does not match this repository")
	}
	_, attempt, ok := strings.Cut(m.Tag[strings.LastIndex(m.Tag, "-")+1:], ".")
	if old := legacyTagPattern.FindStringSubmatch(m.Tag); old != nil {
		attempt, ok = old[2], true
	}
	if singletonTagPattern.MatchString(m.Tag) || toolboxDatePattern.MatchString(m.Tag) {
		attempt, ok = m.RunAttempt, true
	}
	if !ok || !runIDPattern.MatchString(attempt) {
		return nil, fmt.Errorf("published attempt missing from tag")
	}
	var run struct {
		ID         int64  `json:"id"`
		Attempt    int64  `json:"run_attempt"`
		Commit     string `json:"head_sha"`
		Branch     string `json:"head_branch"`
		Conclusion string `json:"conclusion"`
	}
	if err = g.get(ctx, "/actions/runs/"+m.RunID+"/attempts/"+attempt, &run); err != nil {
		return nil, err
	}
	if strconv.FormatInt(run.ID, 10) != m.RunID || strconv.FormatInt(run.Attempt, 10) != attempt || run.Commit != m.Commit || run.Branch != branch || run.Conclusion != "success" {
		return nil, fmt.Errorf("legacy publication run identity or success mismatch")
	}
	artifactID := int64(0)
	for page := 1; page <= 10; page++ {
		var list struct {
			Artifacts []struct {
				ID      int64
				Name    string
				Expired bool
			}
		}
		if err = g.get(ctx, "/actions/runs/"+m.RunID+"/artifacts?per_page=100&page="+strconv.Itoa(page), &list); err != nil {
			return nil, err
		}
		for _, artifact := range list.Artifacts {
			if artifact.Name == BundleForMetadata(m).Artifact+"-"+attempt {
				if artifact.Expired || artifact.ID < 1 || artifactID != 0 {
					return nil, fmt.Errorf("legacy published lock artifact expired or ambiguous")
				}
				artifactID = artifact.ID
			}
		}
		if len(list.Artifacts) < 100 {
			break
		}
		if page == 10 {
			return nil, fmt.Errorf("legacy artifact search exceeds limit")
		}
	}
	if artifactID == 0 {
		return nil, fmt.Errorf("legacy published lock artifact missing")
	}
	r, err := g.request(ctx, "/actions/artifacts/"+strconv.FormatInt(artifactID, 10)+"/zip")
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("legacy artifact download returned HTTP %d", r.StatusCode)
	}
	b, err := signedDownload(ctx, g.client, r.Header.Get("Location"))
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("invalid legacy lock archive")
	}
	var lock []byte
	for _, file := range archive.File {
		// Older published archives retain their original filename.
		if file.Name == "packages.lock.json" || file.Name == "tools.lock.json" {
			if lock != nil {
				return nil, fmt.Errorf("duplicate lock in legacy archive")
			}
			f, err := file.Open()
			if err != nil {
				return nil, err
			}
			lock, err = readBounded(f)
			f.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	if lock == nil {
		return nil, fmt.Errorf("legacy artifact has no lock")
	}
	return lock, nil
}
