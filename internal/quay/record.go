package quay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"bundledock/internal/toolpublish"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Bundle is a published bundle and the Quay repository and aliases it uses.
type Bundle struct {
	Name       string
	Repository string
	Aliases    []string
	// Baseline is the alias that marks the current release ("stable", or "current" for node-current).
	Baseline string
}

// Bundles lists every known bundle, in the order of the publication command, nixbox last.
func Bundles() []Bundle {
	var result []Bundle
	for _, name := range append(append([]string{}, toolpublish.CanonicalBundles...), "nixbox") {
		b, _ := toolpublish.SelectBundle(name)
		result = append(result, Bundle{b.Name, b.Repository, b.PublicationAliases(), b.BaselineAlias()})
	}
	return result
}

// FindBundle returns the named bundle.
func FindBundle(name string) (Bundle, error) {
	for _, b := range Bundles() {
		if b.Name == name {
			return b, nil
		}
	}
	return Bundle{}, fmt.Errorf("unknown bundle %q", name)
}

// Record is the committed publication record docs/<bundle>-published.json.
type Record struct {
	Bundle string `json:"bundle"`
	Image  string `json:"image"`
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
}

// LoadRecord reads the publication record of a bundle below root. It returns
// nil without an error when the bundle has not been published yet.
func LoadRecord(root, bundle string) (*Record, error) {
	raw, err := os.ReadFile(filepath.Join(root, "docs", bundle+"-published.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("invalid publication record for %s", bundle)
	}
	if record.Bundle != bundle || record.Tag == "" || !digestPattern.MatchString(record.Digest) {
		return nil, fmt.Errorf("invalid publication record for %s", bundle)
	}
	return &record, nil
}
