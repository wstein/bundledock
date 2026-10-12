package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"encoding/json"
	"fmt"
)

func (r resolver) gitTrim(ctx context.Context, rel release) (map[string]toolconfig.Spec, error) {
	if rel.Tag != "v0.4.3" {
		return nil, fmt.Errorf("git-trim %s needs a new source recipe; source review required", rel.Tag)
	}
	b, _, err := r.metadata(ctx, "https://api.github.com/repos/foriequal0/git-trim/git/ref/tags/v0.4.3")
	if err != nil {
		return nil, err
	}
	var ref struct {
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal(b, &ref); err != nil {
		return nil, err
	}
	// This reviewed upstream tag points directly at a commit. Any retagging or
	// conversion to an annotated tag must be reviewed rather than guessed.
	if ref.Object.Type != "commit" || ref.Object.SHA != toolconfig.GitTrimCommit {
		return nil, fmt.Errorf("git-trim release tag changed; source review required")
	}
	s := toolconfig.GitTrimSpec()
	b, _, err = r.metadata(ctx, s.URL)
	if err != nil {
		return nil, err
	}
	if _, err := toolconfig.ReadGitTrimArchive(b, s.SHA256); err != nil {
		return nil, err
	}
	return map[string]toolconfig.Spec{"amd64": s, "arm64": s}, nil
}
