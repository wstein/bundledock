package quay

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"time"
)

var (
	versionPattern = regexp.MustCompile(`^(?:\d{4}\.\d{2}\.\d{2}-b[1-9]\d*(?:\.[1-9]\d*)?|(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)-b[1-9]\d*)$`)
	stagingPattern = regexp.MustCompile(`^staging-(.+)-(amd64|arm64)$`)
)

// IsVersion reports whether name is an immutable version tag.
func IsVersion(name string) bool { return versionPattern.MatchString(name) }

// IsStaging reports whether name is an architecture staging tag.
func IsStaging(name string) bool { return stagingPattern.MatchString(name) }

// Inventory fetches and caches the tags of repositories. A repository that
// does not exist yet (HTTP 404 or 401) counts as empty when the bundle is
// not published and manual-only (as scripts/staging-cleanup.py allows).
type Inventory struct {
	Client *Client
	cache  map[string][]Tag
}

// Tags returns the tags of the bundle's repository.
func (i *Inventory) Tags(ctx context.Context, b Bundle, published bool) ([]Tag, error) {
	if tags, ok := i.cache[b.Repository]; ok {
		return tags, nil
	}
	tags, err := i.Client.Tags(ctx, b.Repository, "")
	var status *StatusError
	if err != nil && b.ManualOnly && !published && errors.As(err, &status) && (status.Code == 404 || status.Code == 401) {
		tags, err = []Tag{}, nil
	}
	if err != nil {
		return nil, err
	}
	if i.cache == nil {
		i.cache = map[string][]Tag{}
	}
	i.cache[b.Repository] = tags
	return tags, nil
}

// ListRow is one bundle of the list command.
type ListRow struct {
	Bundle     string `json:"bundle"`
	Repository string `json:"repository"`
	Published  bool   `json:"published"`
	Version    string `json:"version"`
	Digest     string `json:"digest"`
	Staging    int    `json:"staging_tags"`
}

// Find returns the tag with the given name.
func Find(tags []Tag, name string) (Tag, bool) {
	for _, t := range tags {
		if t.Name == name {
			return t, true
		}
	}
	return Tag{}, false
}

// Newest returns the newest version tag of the bundle: the version that the
// baseline alias points to, else the most recently modified version tag.
func Newest(tags []Tag, b Bundle) (Tag, bool) {
	var versions []Tag
	for _, t := range tags {
		if IsVersion(t.Name) {
			versions = append(versions, t)
		}
	}
	modified := func(t Tag) time.Time { m, _ := t.Modified(); return m }
	sort.SliceStable(versions, func(i, j int) bool { return modified(versions[i]).After(modified(versions[j])) })
	if alias, ok := Find(tags, b.Baseline); ok {
		for _, t := range versions {
			if t.ManifestDigest == alias.ManifestDigest {
				return t, true
			}
		}
	}
	if len(versions) > 0 {
		return versions[0], true
	}
	return Tag{}, false
}

// List builds one row per bundle.
func List(ctx context.Context, inv *Inventory, root string) ([]ListRow, error) {
	var rows []ListRow
	for _, b := range Bundles() {
		record, err := LoadRecord(root, b.Name)
		if err != nil {
			return nil, err
		}
		tags, err := inv.Tags(ctx, b, record != nil)
		if err != nil {
			return nil, err
		}
		row := ListRow{Bundle: b.Name, Repository: b.Repository, Published: record != nil}
		for _, t := range tags {
			if IsStaging(t.Name) {
				row.Staging++
			}
		}
		if t, ok := Newest(tags, b); ok {
			row.Version, row.Digest = t.Name, t.ManifestDigest
		}
		rows = append(rows, row)
	}
	return rows, nil
}
