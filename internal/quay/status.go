package quay

import "context"

// Check is one comparison of Quay with the publication record.
type Check struct {
	// Kind is "version", "alias" or "residue".
	Kind     string `json:"kind"`
	Tag      string `json:"tag"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	OK       bool   `json:"ok"`
}

// StatusRow is the result for one bundle.
type StatusRow struct {
	Bundle string `json:"bundle"`
	// State is "ok", "differs" or "skipped" (not published yet).
	State  string  `json:"state"`
	Tag    string  `json:"tag,omitempty"`
	Digest string  `json:"digest,omitempty"`
	Checks []Check `json:"checks,omitempty"`
}

// Compare checks the tags of one bundle against its publication record.
func Compare(b Bundle, record *Record, tags []Tag) StatusRow {
	row := StatusRow{Bundle: b.Name, State: "skipped"}
	if record == nil {
		return row
	}
	row.State, row.Tag, row.Digest = "ok", record.Tag, record.Digest
	check := func(kind, name string) {
		c := Check{Kind: kind, Tag: name, Expected: record.Digest}
		if t, ok := Find(tags, name); ok {
			c.Actual = t.ManifestDigest
		}
		c.OK = c.Actual == c.Expected
		row.Checks = append(row.Checks, c)
	}
	check("version", record.Tag)
	for _, alias := range b.Aliases {
		check("alias", alias)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		name := "staging-" + record.Tag + "-" + arch
		if t, ok := Find(tags, name); ok {
			row.Checks = append(row.Checks, Check{Kind: "residue", Tag: name, Actual: t.ManifestDigest})
		}
	}
	for _, c := range row.Checks {
		if !c.OK || c.Kind == "residue" {
			row.State = "differs"
		}
	}
	return row
}

// Status compares the named bundles (all when names is empty) with their records.
func Status(ctx context.Context, inv *Inventory, root string, bundles []Bundle) ([]StatusRow, error) {
	var rows []StatusRow
	for _, b := range bundles {
		record, err := LoadRecord(root, b.Name)
		if err != nil {
			return nil, err
		}
		if record == nil {
			rows = append(rows, Compare(b, nil, nil))
			continue
		}
		tags, err := inv.Tags(ctx, b, true)
		if err != nil {
			return nil, err
		}
		rows = append(rows, Compare(b, record, tags))
	}
	return rows, nil
}
