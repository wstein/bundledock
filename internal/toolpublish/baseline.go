package toolpublish

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

var ErrBaselineAbsent = errors.New("published stable baseline absent")

const platformsLabel = "io.bundledock.tools.platforms-sha256"

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var legacyTagPattern = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}-([1-9]\d*)-([1-9]\d*)$`)

// ToolDigest includes every pinned tool/asset field on both architectures, and
// the pinned toolchains and extra sources of source builds, but
// excludes input range and catalog digests that can change without tool updates.
func ToolDigest(data []byte) (string, error) {
	var lock toolconfig.Lock
	if err := toolconfig.Decode(bytes.NewReader(data), &lock); err != nil {
		return "", err
	}
	if lock.Schema != 2 || len(lock.Platforms) != 2 || len(lock.Platforms["amd64"]) == 0 || len(lock.Platforms["amd64"]) != len(lock.Platforms["arm64"]) {
		return "", fmt.Errorf("complete schema-2 tool lock required")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for name, spec := range lock.Platforms[arch] {
			if _, ok := lock.Platforms["amd64"][name]; !ok {
				return "", fmt.Errorf("tool inventory differs across architectures")
			}
			if err := toolconfig.ValidateSpec(spec); err != nil {
				return "", err
			}
			if spec.URL == "" {
				return "", fmt.Errorf("tool asset URL required")
			}
		}
	}
	for tc, arches := range lock.Toolchains {
		for arch, spec := range arches {
			if err := toolconfig.ValidateSpec(spec); err != nil || spec.URL == "" {
				return "", fmt.Errorf("invalid toolchain pin %s/%s", tc, arch)
			}
		}
	}
	// A lock without source builds hashes its platforms alone, as before toolchains
	// and extra sources existed, so every earlier digest stays valid.
	var pinned any = lock.Platforms
	if len(lock.Toolchains) != 0 || len(lock.ExtraSources) != 0 {
		pinned = struct {
			Platforms    map[string]map[string]toolconfig.Spec `json:"platforms"`
			Toolchains   map[string]map[string]toolconfig.Spec `json:"toolchains,omitempty"`
			ExtraSources map[string]toolconfig.LockedSource    `json:"extra_sources,omitempty"`
		}{lock.Platforms, lock.Toolchains, lock.ExtraSources}
	}
	b, err := json.Marshal(pinned)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

func readBounded(body io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(body, (8<<20)+1))
	if err != nil || len(b) > 8<<20 {
		return nil, fmt.Errorf("publication metadata exceeds limit or cannot be read")
	}
	return b, nil
}

// signedDownload follows one service-issued HTTPS location without forwarding
// any credentials. It never follows a second redirect or exposes signed URLs.
func signedDownload(ctx context.Context, client *http.Client, location string) ([]byte, error) {
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Port() != "" {
		return nil, fmt.Errorf("invalid metadata download location")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata download request")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metadata download failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata download returned HTTP %d", response.StatusCode)
	}
	return readBounded(response.Body)
}

// PublishedToolDigest uses stable's immutable config labels as the durable
// baseline. Legacy publications require their exact, hash-bound Actions lock.
// Missing stable or missing/expired artifacts are errors, never a bootstrap.
func PublishedToolDigest(ctx context.Context, client *http.Client, image, username, password string, legacy func(Metadata) ([]byte, error)) (string, error) {
	return PublishedToolDigestForBundle(ctx, client, bundleForImage(image).Name, image, username, password, legacy)
}

func PublishedToolDigestForBundle(ctx context.Context, client *http.Client, name, image, username, password string, legacy func(Metadata) ([]byte, error)) (string, error) {
	bundle, err := SelectBundle(name)
	if err != nil {
		return "", err
	}
	if err = validateBundleImage(bundle, image); err != nil {
		return "", err
	}
	alias := bundle.BaselineAlias()

	registry, err := newQuayReader(ctx, client, image, username, password)
	if err != nil {
		return "", err
	}
	get := func(kind, ref string) ([]byte, error) {
		response, err := registry.request(ctx, http.MethodGet, kind, ref)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		var b []byte
		if kind == "blobs" && response.StatusCode == http.StatusFound {
			b, err = signedDownload(ctx, registry.client, response.Header.Get("Location"))
		} else if response.StatusCode == http.StatusOK {
			b, err = readBounded(response.Body)
		} else {
			if kind == "manifests" && ref == alias && response.StatusCode == http.StatusNotFound {
				return nil, ErrBaselineAbsent
			}
			return nil, fmt.Errorf("published baseline %s returned HTTP %d", kind, response.StatusCode)
		}
		if err != nil {
			return nil, err
		}
		if ref != alias && fmt.Sprintf("sha256:%x", sha256.Sum256(b)) != ref {
			return nil, fmt.Errorf("published baseline content digest mismatch")
		}
		return b, nil
	}
	b, err := get("manifests", alias)
	if err != nil {
		return "", err
	}
	var index struct {
		Manifests []struct {
			Digest   string
			Platform struct{ OS, Architecture string }
		}
	}
	if err = json.Unmarshal(b, &index); err != nil {
		return "", err
	}
	if len(index.Manifests) != 2 {
		return "", fmt.Errorf("stable must contain both native platforms")
	}
	seen := map[string]bool{}
	var identity Metadata
	semantic := ""
	for i, platform := range index.Manifests {
		arch := platform.Platform.Architecture
		if platform.Platform.OS != "linux" || (arch != "amd64" && arch != "arm64") || seen[arch] || !digestPattern.MatchString(platform.Digest) {
			return "", fmt.Errorf("invalid published baseline platform")
		}
		seen[arch] = true
		b, err = get("manifests", platform.Digest)
		if err != nil {
			return "", err
		}
		var manifest struct{ Config struct{ Digest string } }
		if err = json.Unmarshal(b, &manifest); err != nil {
			return "", err
		}
		if !digestPattern.MatchString(manifest.Config.Digest) {
			return "", fmt.Errorf("invalid published config digest")
		}
		b, err = get("blobs", manifest.Config.Digest)
		if err != nil {
			return "", err
		}
		var config struct {
			Architecture, OS string
			Config           struct{ Labels map[string]string }
		}
		if err = json.Unmarshal(b, &config); err != nil {
			return "", err
		}
		if config.Architecture != arch || config.OS != "linux" {
			return "", fmt.Errorf("published config platform mismatch")
		}
		l := config.Config.Labels
		m := Metadata{Bundle: l["io.bundledock.bundle"], Image: image, RunID: l["io.bundledock.github.run-id"], RunAttempt: l["io.bundledock.github.run-attempt"], Tag: l["org.opencontainers.image.version"], Commit: l["org.opencontainers.image.revision"], Source: l["org.opencontainers.image.source"], Created: l["org.opencontainers.image.created"], LockSHA256: l["io.bundledock.tools.lock-sha256"]}
		if m.Bundle == "" && (bundle.Name == "node-current" || (bundle.Repository == "node" && singletonTagPattern.MatchString(m.Tag))) {
			return "", fmt.Errorf("Node channel baseline source identity missing")
		}
		if BundleForMetadata(m).Name != bundle.Name {
			return "", fmt.Errorf("published baseline belongs to another source bundle")
		}
		old := legacyTagPattern.FindStringSubmatch(m.Tag)
		if m.RunID == "" && old != nil {
			m.RunID = old[1]
		}
		if !runIDPattern.MatchString(m.RunID) || (!tagPattern.MatchString(m.Tag) && old == nil) || !commitPattern.MatchString(m.Commit) || !hashPattern.MatchString(m.LockSHA256) {
			return "", fmt.Errorf("published baseline lacks valid provenance")
		}
		if (singletonTagPattern.MatchString(m.Tag) || toolboxDatePattern.MatchString(m.Tag)) && (!runIDPattern.MatchString(m.RunAttempt) || (bundleForImage(image).Name == "toolbox" && !toolboxDatePattern.MatchString(m.Tag))) {
			return "", fmt.Errorf("published packaging tag or workflow attempt invalid")
		}
		digest := l[platformsLabel]
		if digest != "" && !hashPattern.MatchString(digest) {
			return "", fmt.Errorf("invalid published tool digest")
		}
		if i == 0 {
			identity, semantic = m, digest
		} else if m != identity || digest != semantic {
			return "", fmt.Errorf("published platforms disagree on baseline")
		}
	}
	if semantic != "" {
		return semantic, nil
	}
	if legacy == nil {
		return "", fmt.Errorf("legacy publication requires its verified lock artifact")
	}
	b, err = legacy(identity)
	if err != nil {
		return "", fmt.Errorf("legacy published lock unavailable: %w", err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != identity.LockSHA256 {
		return "", fmt.Errorf("legacy artifact differs from published lock")
	}
	return ToolDigest(b)
}
