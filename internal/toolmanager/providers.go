package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var errNoRelease = errors.New("no matching GitHub release")

type resolver struct{ client *http.Client }
type asset struct {
	Name          string `json:"name"`
	URL           string `json:"browser_download_url"`
	Digest        string `json:"digest,omitempty"`
	DigestPresent bool   `json:"-"`
}

func (a *asset) UnmarshalJSON(data []byte) error {
	type plain asset
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	rawDigest, exists := fields["digest"]
	// GitHub uses null for older assets without a published digest. An empty
	// string remains present and invalid rather than enabling the fallback.
	v.DigestPresent = exists && strings.TrimSpace(string(rawDigest)) != "null"
	*a = asset(v)
	return nil
}

type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

func (r resolver) get(ctx context.Context, raw string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "bundledock-tools")
	if req.URL.Scheme == "https" && req.URL.Host == "api.github.com" {
		if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	client := *r.client
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many upstream redirects")
		}
		origin := via[0].URL.Hostname()
		host := next.URL.Hostname()
		officialCDN := origin == "github.com" && (host == "release-assets.githubusercontent.com" || host == "objects.githubusercontent.com")
		if next.URL.Scheme != "https" || next.URL.User != nil || next.URL.Port() != "" || host != origin && !officialCDN {
			return fmt.Errorf("unsafe upstream redirect")
		}
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %s", raw, resp.Status)
	}
	return resp, nil
}
func (r resolver) metadata(ctx context.Context, raw string) ([]byte, http.Header, error) {
	resp, err := r.get(ctx, raw)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return nil, nil, err
	}
	if len(b) > 8<<20 {
		return nil, nil, fmt.Errorf("metadata too large")
	}
	return b, resp.Header, nil
}
func expand(pattern, version, arch string) string {
	return strings.NewReplacer("{version}", version, "{arch}", arch).Replace(pattern)
}
func (r resolver) github(ctx context.Context, p toolconfig.Package, c toolconfig.Constraint) (release, error) {
	next := "https://api.github.com/repos/" + p.Repository + "/releases?per_page=100&page=1"
	seen := map[string]bool{}
	var best release
	var bestVersion toolconfig.Version
	for next != "" {
		if seen[next] || len(seen) >= 100 {
			return best, fmt.Errorf("invalid or excessive GitHub pagination")
		}
		seen[next] = true
		b, headers, err := r.metadata(ctx, next)
		if err != nil {
			return best, err
		}
		var page []release
		if err := json.Unmarshal(b, &page); err != nil {
			return best, err
		}
		for _, rel := range page {
			if rel.Draft || rel.Prerelease || !strings.HasPrefix(rel.Tag, p.TagPrefix) {
				continue
			}
			version, err := p.Version(rel.Tag)
			if err != nil || !c.Contains(version) {
				continue
			}
			v, _ := toolconfig.ParseVersion(version)
			if best.Tag == "" || v.Compare(bestVersion) > 0 {
				best, bestVersion = rel, v
			}
		}
		next = ""
		for _, link := range strings.Split(headers.Get("Link"), ",") {
			parts := strings.Split(link, ";")
			if len(parts) < 2 || strings.TrimSpace(parts[1]) != `rel="next"` {
				continue
			}
			candidate := strings.Trim(strings.TrimSpace(parts[0]), "<>")
			u, err := url.Parse(candidate)
			if err != nil || u.Scheme != "https" || u.Host != "api.github.com" || u.Path != "/repos/"+p.Repository+"/releases" || u.User != nil {
				return best, fmt.Errorf("unsafe GitHub pagination URL")
			}
			next = candidate
		}
	}
	if best.Tag == "" {
		return best, fmt.Errorf("%w satisfies %s", errNoRelease, c.String())
	}
	return best, nil
}
func findAsset(rel release, name string) (asset, error) {
	var found asset
	for _, a := range rel.Assets {
		if a.Name == name {
			if found.URL != "" {
				return found, fmt.Errorf("ambiguous asset %s", name)
			}
			found = a
		}
	}
	if found.URL == "" {
		return found, fmt.Errorf("latest matching release %s lacks asset %s", rel.Tag, name)
	}
	return found, nil
}
func parseSidecar(b []byte, asset string) (string, error) {
	var found string
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 1 || len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			digest := fields[0]
			decoded, err := hex.DecodeString(digest)
			if err != nil || len(decoded) != 32 {
				continue
			}
			if found != "" {
				return "", fmt.Errorf("ambiguous checksum for %s", asset)
			}
			found = strings.ToLower(digest)
		}
	}
	if found == "" {
		return "", fmt.Errorf("missing SHA256 checksum for %s", asset)
	}
	return found, nil
}
func (r resolver) pin(ctx context.Context, raw string, hosts []string, expected, provenance string) (toolconfig.Spec, error) {
	var s toolconfig.Spec
	s.URL = raw
	if err := toolconfig.ValidateURL(raw, hosts); err != nil {
		return s, err
	}
	h := sha256.New()
	var verify hash.Hash
	if expected != "" {
		switch provenance {
		case "publisher-sha512":
			verify = sha512.New()
		default:
			verify = sha256.New()
		}
		decoded, err := hex.DecodeString(expected)
		if err != nil || len(decoded) != verify.Size() {
			return s, fmt.Errorf("invalid publisher checksum")
		}
	}
	resp, err := r.get(ctx, raw)
	if err != nil {
		return s, err
	}
	writer := io.Writer(h)
	if verify != nil {
		writer = io.MultiWriter(h, verify)
	}
	_, copyErr := io.Copy(writer, resp.Body)
	closeErr := resp.Body.Close()
	if copyErr != nil {
		return s, copyErr
	}
	if closeErr != nil {
		return s, closeErr
	}
	if verify != nil && !strings.EqualFold(hex.EncodeToString(verify.Sum(nil)), expected) {
		return s, fmt.Errorf("publisher checksum mismatch for %s", raw)
	}
	s.SHA256 = hex.EncodeToString(h.Sum(nil))
	if provenance == "publisher-sha512" {
		s.SHA256 = ""
		s.SHA512 = hex.EncodeToString(verify.Sum(nil))
	}
	s.Verification = provenance
	if expected == "" {
		s.Verification = "tls-download-sha256"
	}
	return s, nil
}
func (r resolver) resolve(ctx context.Context, p toolconfig.Package, c toolconfig.Constraint) (map[string]toolconfig.Spec, error) {
	if p.Provider == "antigravity" {
		return r.antigravity(ctx, p, c)
	}
	rel, err := r.github(ctx, p, c)
	if err != nil && !(p.Provider == "claude" && errors.Is(err, errNoRelease)) {
		return nil, err
	}
	version, _ := p.Version(rel.Tag)
	specs := map[string]toolconfig.Spec{}
	if p.Provider == "claude" {
		latest, _, err := r.metadata(ctx, p.BaseURL+"/latest")
		if err != nil {
			return nil, err
		}
		current := strings.TrimSpace(string(latest))
		v, err := toolconfig.ParseVersion(current)
		if err != nil {
			return nil, err
		}
		old, _ := toolconfig.ParseVersion(version)
		if c.Contains(current) && (version == "" || v.Compare(old) > 0) {
			version = current
		}
		if version == "" {
			return nil, fmt.Errorf("no stable Claude release satisfies %s", c.String())
		}
		b, _, err := r.metadata(ctx, p.BaseURL+"/"+version+"/manifest.json")
		if err != nil {
			return nil, err
		}
		var manifest struct {
			Version   string `json:"version"`
			Platforms map[string]struct {
				Checksum string `json:"checksum"`
			} `json:"platforms"`
		}
		if err := json.Unmarshal(b, &manifest); err != nil {
			return nil, err
		}
		if manifest.Version != "" && manifest.Version != version {
			return nil, fmt.Errorf("Claude manifest version mismatch")
		}
		for _, arch := range []string{"amd64", "arm64"} {
			platform := p.Architectures[arch]
			sum := manifest.Platforms[platform].Checksum
			if sum == "" {
				return nil, fmt.Errorf("latest matching Claude version lacks %s checksum", platform)
			}
			s, err := r.pin(ctx, p.BaseURL+"/"+version+"/"+platform+"/"+p.Asset, p.AllowedHosts, sum, "publisher-sha256")
			if err != nil {
				return nil, err
			}
			s.Version = version
			specs[arch] = s
		}
		return specs, nil
	}
	return r.githubAssets(ctx, p, rel)
}
func (r resolver) githubAssets(ctx context.Context, p toolconfig.Package, rel release) (map[string]toolconfig.Spec, error) {
	specs := map[string]toolconfig.Spec{}
	version, _ := p.Version(rel.Tag)
	if p.Normalization == "tmux-release" {
		version = strings.TrimPrefix(rel.Tag, p.TagPrefix)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		name := expand(p.Asset, version, p.Architectures[arch])
		a, err := findAsset(rel, name)
		if err != nil {
			return nil, err
		}
		expected := ""
		if a.DigestPresent || a.Digest != "" {
			if !strings.HasPrefix(a.Digest, "sha256:") {
				return nil, fmt.Errorf("unsupported asset digest %q", a.Digest)
			}
			expected = strings.TrimPrefix(a.Digest, "sha256:")
			decoded, err := hex.DecodeString(expected)
			if err != nil || len(decoded) != 32 {
				return nil, fmt.Errorf("invalid published asset digest")
			}
		} else if p.Sidecar != "" {
			sidecar := strings.ReplaceAll(expand(p.Sidecar, version, p.Architectures[arch]), "{asset}", name)
			for _, sa := range rel.Assets {
				if sa.Name == sidecar {
					if err := toolconfig.ValidateURL(sa.URL, p.AllowedHosts); err != nil {
						return nil, err
					}
					b, _, err := r.metadata(ctx, sa.URL)
					if err != nil {
						return nil, err
					}
					expected, err = parseSidecar(b, name)
					if err != nil {
						return nil, err
					}
					break
				}
			}
		}
		s, err := r.pin(ctx, a.URL, p.AllowedHosts, expected, "publisher-sha256")
		if err != nil {
			return nil, err
		}
		s.Version = rel.Tag
		specs[arch] = s
	}
	return specs, nil
}

// The official updater may publish before GitHub. Union current manifests with
// historical GitHub releases, then require the chosen version on both platforms.
func (r resolver) antigravity(ctx context.Context, p toolconfig.Package, c toolconfig.Constraint) (map[string]toolconfig.Spec, error) {
	rel, err := r.github(ctx, p, c)
	if err != nil && !errors.Is(err, errNoRelease) {
		return nil, err
	}
	type current struct {
		Version string `json:"version"`
		URL     string `json:"url"`
		SHA512  string `json:"sha512"`
	}
	manifests := map[string]current{}
	best := rel.Tag
	for _, arch := range []string{"amd64", "arm64"} {
		b, _, err := r.metadata(ctx, p.MetadataURL+"/manifests/linux_"+arch+".json")
		if err != nil {
			return nil, err
		}
		var m current
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		v, err := toolconfig.ParseVersion(m.Version)
		if err != nil {
			return nil, err
		}
		manifests[arch] = m
		if c.Contains(m.Version) {
			old, _ := toolconfig.ParseVersion(best)
			if best == "" || v.Compare(old) > 0 {
				best = m.Version
			}
		}
	}
	if best == "" {
		return nil, fmt.Errorf("no stable Antigravity release satisfies %s", c.String())
	}
	completeCurrent := true
	for _, arch := range []string{"amd64", "arm64"} {
		m := manifests[arch]
		if m.Version != best || m.URL == "" || m.SHA512 == "" {
			completeCurrent = false
		}
	}
	if best == rel.Tag && !completeCurrent {
		return r.githubAssets(ctx, p, rel)
	}
	specs := map[string]toolconfig.Spec{}
	for _, arch := range []string{"amd64", "arm64"} {
		m := manifests[arch]
		if m.Version != best || m.SHA512 == "" {
			return nil, fmt.Errorf("latest matching Antigravity version %s lacks %s manifest", best, arch)
		}
		s, err := r.pin(ctx, m.URL, p.AllowedHosts, m.SHA512, "publisher-sha512")
		if err != nil {
			return nil, err
		}
		s.Version = best
		specs[arch] = s
	}
	return specs, nil
}
