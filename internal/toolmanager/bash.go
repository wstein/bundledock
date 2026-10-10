package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"fmt"
	"regexp"
	"strings"
)

var bashArchiveLink = regexp.MustCompile(`href=["']bash-([0-9]+\.[0-9]+)\.tar\.gz["']`)

// GNU publishes a base tarball and an ordered patch series separately. Compare
// effective patched versions, then pin each input once for both architectures.
func (r resolver) bash(ctx context.Context, p toolconfig.Package, c toolconfig.Constraint) (map[string]toolconfig.Spec, error) {
	index, _, err := r.metadata(ctx, p.BaseURL+"/")
	if err != nil {
		return nil, err
	}
	var best string
	var bestVersion toolconfig.Version
	seen := map[string]bool{}
	for _, match := range bashArchiveLink.FindAllSubmatch(index, -1) {
		base := string(match[1])
		if seen[base] {
			continue
		}
		seen[base] = true
		v, err := toolconfig.ParseVersion(base + ".0")
		if err != nil {
			continue
		}
		if v[0] > 99 || v[1] > 99 {
			continue
		}
		upper := c.Upper
		if c.Exact {
			upper = c.Lower
		}
		if v[0] > upper[0] || v[0] == upper[0] && v[1] > upper[1] {
			continue
		}
		// An older minor cannot satisfy a lower bound regardless of patch level.
		if v[0] < c.Lower[0] || v[0] == c.Lower[0] && v[1] < c.Lower[1] {
			continue
		}
		count := 0
		if strings.Contains(string(index), `href="bash-`+base+`-patches/"`) || strings.Contains(string(index), `href='bash-`+base+`-patches/'`) {
			page, _, err := r.metadata(ctx, p.BaseURL+"/bash-"+base+"-patches/")
			if err != nil {
				return nil, err
			}
			_, prefix, _, _ := toolconfig.BashBase(base + ".0")
			links := regexp.MustCompile(`href=["']` + regexp.QuoteMeta(prefix) + `-([0-9]{3})["']`)
			numbers := map[int]bool{}
			for _, m := range links.FindAllSubmatch(page, -1) {
				n := int(m[1][0]-'0')*100 + int(m[1][1]-'0')*10 + int(m[1][2]-'0')
				if n == 0 {
					return nil, fmt.Errorf("invalid GNU patch zero")
				}
				numbers[n] = true
				if n > count {
					count = n
				}
			}
			if len(numbers) != count {
				return nil, fmt.Errorf("GNU Bash patch series has a gap")
			}
		}
		for n := count; n >= 0; n-- {
			v[2] = uint64(n)
			if c.Contains(v.String()) {
				if best == "" || v.Compare(bestVersion) > 0 {
					best, bestVersion = v.String(), v
				}
				break
			}
		}
	}
	if best == "" {
		return nil, fmt.Errorf("no GNU Bash release satisfies %s", c.String())
	}
	base, prefix, count, _ := toolconfig.BashBase(best)
	s, err := r.pin(ctx, p.BaseURL+"/bash-"+base+".tar.gz", p.AllowedHosts, "", "tls-download-sha256")
	if err != nil {
		return nil, err
	}
	s.Version = best
	for i := 1; i <= count; i++ {
		raw := fmt.Sprintf("%s/bash-%s-patches/%s-%03d", p.BaseURL, base, prefix, i)
		patch, err := r.pin(ctx, raw, p.AllowedHosts, "", "tls-download-sha256")
		if err != nil {
			return nil, err
		}
		s.Patches = append(s.Patches, toolconfig.Input{URL: raw, SHA256: patch.SHA256, Verification: patch.Verification})
	}
	return map[string]toolconfig.Spec{"amd64": s, "arm64": s}, nil
}
