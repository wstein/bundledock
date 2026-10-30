package quay

import (
	"context"
	"sort"
	"time"
)

// InfoRow describes one tag.
type InfoRow struct {
	Tag           string   `json:"tag"`
	Digest        string   `json:"digest"`
	Modified      string   `json:"modified"`
	AgeDays       int      `json:"age_days"`
	Architectures []string `json:"architectures,omitempty"`
	Version       string   `json:"version,omitempty"`
	Revision      string   `json:"revision,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// Info lists all tags of the bundle's repository with registry details.
// A failure to inspect one digest is reported on its rows, not fatal.
func Info(ctx context.Context, c *Client, b Bundle, now time.Time) ([]InfoRow, error) {
	tags, err := c.Tags(ctx, b.Repository, "")
	if err != nil {
		return nil, err
	}
	registry, err := c.Registry(ctx, b.Repository)
	if err != nil {
		return nil, err
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	type result struct {
		image Image
		err   error
	}
	cache := map[string]result{}
	var rows []InfoRow
	for _, t := range tags {
		row := InfoRow{Tag: t.Name, Digest: t.ManifestDigest, Modified: t.LastModified}
		if m, err := t.Modified(); err == nil {
			row.AgeDays = int(now.Sub(m).Hours() / 24)
		}
		res, ok := cache[t.ManifestDigest]
		if !ok {
			res.image, res.err = registry.Inspect(ctx, t.ManifestDigest)
			cache[t.ManifestDigest] = res
		}
		if res.err != nil {
			row.Error = res.err.Error()
		} else {
			row.Architectures, row.Version, row.Revision = res.image.Architectures, res.image.Version, res.image.Revision
		}
		rows = append(rows, row)
	}
	return rows, nil
}
