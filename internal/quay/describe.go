package quay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// MaxDescription is the longest description Quay accepts, in characters.
const MaxDescription = 5000

// SetDescription writes the repository description with an OAuth token
// (Administer Repositories). The token is only ever sent as a bearer header.
func (c *Client) SetDescription(ctx context.Context, repository, token, description string) error {
	if token == "" {
		return fmt.Errorf("QUAY_TOKEN is required")
	}
	if err := c.requireDefaultHost(); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"description": description})
	if err != nil {
		return err
	}
	header := http.Header{"Authorization": {"Bearer " + token}, "Content-Type": {"application/json"}}
	_, err = c.Do(ctx, http.MethodPut, "/api/v1/repository/"+Namespace+"/"+repository, header, body, http.StatusOK)
	return err
}

// DescriptionPath is the reviewed description file of a bundle.
func DescriptionPath(root, bundle string) string {
	return filepath.Join(root, "bundles", bundle, "description.md")
}

// LoadDescription reads bundles/<name>/description.md. It returns false when
// the file does not exist.
func LoadDescription(root, bundle string) (string, bool, error) {
	raw, err := os.ReadFile(DescriptionPath(root, bundle))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	text := strings.TrimSpace(string(raw))
	if !utf8.ValidString(text) || text == "" || utf8.RuneCountInString(text) > MaxDescription {
		return "", false, fmt.Errorf("%s must be valid, not empty and at most %d characters", DescriptionPath(root, bundle), MaxDescription)
	}
	return text, true, nil
}

// DescribeRow is the result for one bundle.
type DescribeRow struct {
	Bundle string `json:"bundle"`
	// State is "same", "differs", "applied" or "skipped" (no description file).
	State  string `json:"state"`
	Wanted string `json:"wanted,omitempty"`
	Live   string `json:"live,omitempty"`
	Public bool   `json:"public"`
}

// Describe compares the wanted descriptions with the live ones. With apply it
// writes the ones that differ; visibility is only checked, never changed.
func Describe(ctx context.Context, c *Client, root string, bundles []Bundle, apply bool, token string) ([]DescribeRow, error) {
	var rows []DescribeRow
	for _, b := range bundles {
		wanted, ok, err := LoadDescription(root, b.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			rows = append(rows, DescribeRow{Bundle: b.Name, State: "skipped"})
			continue
		}
		repo, err := c.Repository(ctx, b.Repository)
		if err != nil {
			return nil, err
		}
		row := DescribeRow{Bundle: b.Name, State: "same", Wanted: wanted, Live: repo.Description, Public: repo.IsPublic}
		if strings.TrimSpace(repo.Description) != wanted {
			row.State = "differs"
			if apply {
				if err := c.SetDescription(ctx, b.Repository, token, wanted); err != nil {
					return nil, err
				}
				row.State = "applied"
			}
		}
		if !repo.IsPublic {
			row.State = "differs"
		}
		rows = append(rows, row)
	}
	return rows, nil
}
