// Package nixbox verifies and assembles the nixbox bundle: a Nix store whose
// paths come only from the signed cache.nixos.org binary cache.
package nixbox

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// CachePublicKey is the cache.nixos.org signing key. It is pinned here as well as
// in the catalog; both must agree. Verified against /etc/nix/nix.conf of the
// digest-pinned nixos/nix image (trusted-public-keys).
const CachePublicKey = "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="

// CacheURL is the only substituter the bundle accepts.
const CacheURL = "https://cache.nixos.org"

// Provider values of a catalog package.
const (
	ProviderNix      = "nix"
	ProviderOfficial = "official"
	RoleRunner       = "runner"
)

// Catalog declares what nixbox contains. The lock pins it by store path.
type Catalog struct {
	Schema    int                `json:"schema"`
	Nixpkgs   Nixpkgs            `json:"nixpkgs"`
	Cache     Cache              `json:"cache"`
	Platforms map[string]string  `json:"platforms"`
	Packages  map[string]Package `json:"packages"`
}

// Nixpkgs pins the package set by revision and unpacked tree hash.
type Nixpkgs struct {
	Rev     string `json:"rev"`
	URL     string `json:"url"`
	NarHash string `json:"narHash"`
}

// Cache names the binary cache and the key its signatures must verify against.
type Cache struct {
	URL       string `json:"url"`
	PublicKey string `json:"public_key"`
}

// Package is one catalog entry. A nix package is delivered as a store path; an
// official package stays a toolbox binary that nixbox only makes runnable.
type Package struct {
	Provider string `json:"provider"`
	// Nix packages.
	Attr string `json:"attr,omitempty"`
	Role string `json:"role,omitempty"`
	// Official packages: the toolbox-relative binary and environment paths.
	Bundle string            `json:"bundle,omitempty"`
	Binary string            `json:"binary,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

var (
	packageName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)
	attrPath    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*(\.[A-Za-z_][A-Za-z0-9_-]*)*$`)
	revPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sriSHA256   = regexp.MustCompile(`^sha256-[A-Za-z0-9+/]{43}=$`)
	envName     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

// Architectures maps the bundle architecture names to Nix systems.
var Architectures = map[string]string{"amd64": "x86_64-linux", "arm64": "aarch64-linux"}

// ParseCatalog decodes and validates a catalog.
func ParseCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("catalog: trailing data")
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	return &c, nil
}

func (c *Catalog) validate() error {
	if c.Schema != 1 {
		return fmt.Errorf("unsupported schema %d", c.Schema)
	}
	if !revPattern.MatchString(c.Nixpkgs.Rev) {
		return fmt.Errorf("nixpkgs rev must be a 40 digit commit hash")
	}
	if !strings.HasPrefix(c.Nixpkgs.URL, "https://releases.nixos.org/nixpkgs/") || !strings.Contains(c.Nixpkgs.URL, c.Nixpkgs.Rev[:12]) {
		return fmt.Errorf("nixpkgs url must be an https://releases.nixos.org/nixpkgs/ release of the pinned revision")
	}
	if !sriSHA256.MatchString(c.Nixpkgs.NarHash) {
		return fmt.Errorf("nixpkgs narHash must be an SRI sha256 hash")
	}
	if c.Cache.URL != CacheURL {
		return fmt.Errorf("cache url must be %s", CacheURL)
	}
	if c.Cache.PublicKey != CachePublicKey {
		return fmt.Errorf("cache public key must be the pinned cache.nixos.org-1 key")
	}
	if len(c.Platforms) != len(Architectures) {
		return fmt.Errorf("platforms must be exactly amd64 and arm64")
	}
	for arch, system := range Architectures {
		if c.Platforms[arch] != system {
			return fmt.Errorf("platform %s must be %s", arch, system)
		}
	}
	if len(c.Packages) == 0 {
		return fmt.Errorf("no packages")
	}
	runners := 0
	official := 0
	for name, p := range c.Packages {
		if !packageName.MatchString(name) {
			return fmt.Errorf("invalid package name %q", name)
		}
		switch p.Provider {
		case ProviderNix:
			if !attrPath.MatchString(p.Attr) {
				return fmt.Errorf("package %s: invalid attribute path %q", name, p.Attr)
			}
			if p.Role != "" && p.Role != RoleRunner {
				return fmt.Errorf("package %s: unknown role %q", name, p.Role)
			}
			if p.Role == RoleRunner {
				runners++
			}
			if p.Bundle != "" || p.Binary != "" || len(p.Env) != 0 {
				return fmt.Errorf("package %s: nix packages take only attr and role", name)
			}
		case ProviderOfficial:
			official++
			if p.Bundle != "toolbox" {
				return fmt.Errorf("package %s: official packages come from the toolbox bundle", name)
			}
			if !relativePath(p.Binary) {
				return fmt.Errorf("package %s: invalid binary path %q", name, p.Binary)
			}
			for key, value := range p.Env {
				if !envName.MatchString(key) || !relativePath(value) {
					return fmt.Errorf("package %s: invalid environment entry %s", name, key)
				}
			}
			if p.Attr != "" || p.Role != "" {
				return fmt.Errorf("package %s: official packages take no attr or role", name)
			}
		default:
			return fmt.Errorf("package %s: unknown provider %q", name, p.Provider)
		}
	}
	if official > 0 {
		for _, want := range []string{"glibc", "gcc-lib"} {
			if p, ok := c.Packages[want]; !ok || p.Provider != ProviderNix || p.Role != RoleRunner {
				return fmt.Errorf("official packages need the runner package %s", want)
			}
		}
	}
	if runners != 0 && runners != 2 {
		return fmt.Errorf("runner packages must be exactly glibc and gcc-lib")
	}
	return nil
}

func relativePath(p string) bool {
	return p != "" && p == path.Clean(p) && !path.IsAbs(p) && p != "." && !strings.HasPrefix(p, "../")
}

// CatalogDigest is the hex SHA-256 of the exact catalog bytes the lock records.
func CatalogDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// NixNames lists the packages delivered as store paths, sorted.
func (c *Catalog) NixNames() []string {
	var names []string
	for name, p := range c.Packages {
		if p.Provider == ProviderNix {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// OfficialNames lists the toolbox binaries nixbox shims, sorted.
func (c *Catalog) OfficialNames() []string {
	var names []string
	for name, p := range c.Packages {
		if p.Provider == ProviderOfficial {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
