package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// node resolves an explicit official LTS or Current channel. A newer release missing either
// native archive fails closed rather than silently falling back.
func (r resolver) node(ctx context.Context, p toolconfig.Package, c toolconfig.Constraint) (map[string]toolconfig.Spec, error) {
	channel := p.Channel
	if channel == "" {
		channel = "lts"
	}
	if channel != "lts" && channel != "current" {
		return nil, fmt.Errorf("invalid Node channel %q", channel)
	}
	b, _, err := r.metadata(ctx, p.MetadataURL)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Version string          `json:"version"`
		LTS     json.RawMessage `json:"lts"`
		Files   []string        `json:"files"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	best := ""
	var bestV toolconfig.Version
	var files []string
	for _, row := range rows {
		var lts string
		isLTS := json.Unmarshal(row.LTS, &lts) == nil && lts != ""
		if (channel == "lts" && !isLTS) || (channel == "current" && string(row.LTS) != "false") {
			continue
		}
		v, err := toolconfig.ParseVersion(strings.TrimPrefix(row.Version, "v"))
		if err != nil || !c.Contains(v.String()) || row.Version != "v"+v.String() {
			continue
		}
		if best == "" || v.Compare(bestV) > 0 {
			best = row.Version
			bestV = v
			files = row.Files
		}
	}
	if best == "" {
		return nil, fmt.Errorf("no official Node %s satisfies %s", channel, c.String())
	}
	for _, a := range []string{"x64", "arm64"} {
		found := false
		for _, f := range files {
			if f == "linux-"+a {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("Node %s lacks linux-%s prebuilt", channel, a)
		}
	}
	base := p.BaseURL + "/" + best + "/"
	sums, _, err := r.metadata(ctx, base+"SHASUMS256.txt")
	if err != nil {
		return nil, err
	}
	out := map[string]toolconfig.Spec{}
	for _, arch := range []string{"amd64", "arm64"} {
		name := expand(p.Asset, strings.TrimPrefix(best, "v"), p.Architectures[arch])
		sum, err := parseSidecar(sums, name)
		if err != nil {
			return nil, err
		}
		s, err := r.pin(ctx, base+name, p.AllowedHosts, sum, "publisher-sha256")
		if err != nil {
			return nil, err
		}
		s.Version = best
		out[arch] = s
	}
	return out, nil
}
