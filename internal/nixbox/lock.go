package nixbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
)

// Lock pins the closure of every nix package per architecture.
type Lock struct {
	Schema        int                     `json:"schema"`
	CatalogDigest string                  `json:"catalog_digest"`
	Platforms     map[string]PlatformLock `json:"platforms"`
}

// PlatformLock is the closure for one architecture. Roots maps a catalog package
// name to its store path; Paths holds every path of the combined closure.
type PlatformLock struct {
	System string              `json:"system"`
	Roots  map[string]string   `json:"roots"`
	Paths  map[string]PathInfo `json:"paths"`
}

var storePathPattern = regexp.MustCompile(`^/nix/store/[0-9a-df-np-sv-z]{32}-[A-Za-z0-9+._?=-]+$`)

// ParseLock decodes a lock strictly.
func ParseLock(data []byte) (*Lock, error) {
	var l Lock
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("lock: trailing data")
	}
	return &l, nil
}

// CheckLock verifies a lock against its catalog without any network access: the
// catalog digest, roots, a reference-closed and minimal closure, well-formed
// hashes, and one valid signature by key on every path. publicKey is the
// catalog's cache key in production; tests pass their own.
func CheckLock(c *Catalog, catalogData []byte, l *Lock, publicKey string) error {
	if l.Schema != 1 {
		return fmt.Errorf("lock: unsupported schema %d", l.Schema)
	}
	if l.CatalogDigest != CatalogDigest(catalogData) {
		return fmt.Errorf("lock: catalog digest does not match the catalog; refresh the lock")
	}
	if len(l.Platforms) != len(c.Platforms) {
		return fmt.Errorf("lock: platforms differ from the catalog")
	}
	for arch, system := range c.Platforms {
		p, ok := l.Platforms[arch]
		if !ok {
			return fmt.Errorf("lock: missing platform %s", arch)
		}
		if err := checkPlatform(c, arch, system, p, publicKey); err != nil {
			return fmt.Errorf("lock %s: %w", arch, err)
		}
	}
	return nil
}

func checkPlatform(c *Catalog, arch, system string, p PlatformLock, publicKey string) error {
	if p.System != system {
		return fmt.Errorf("system %q, catalog says %q", p.System, system)
	}
	names := c.NixNames()
	if len(p.Roots) != len(names) {
		return fmt.Errorf("roots differ from the catalog's nix packages")
	}
	for _, name := range names {
		if _, ok := p.Roots[name]; !ok {
			return fmt.Errorf("no root for %s", name)
		}
	}
	reachable := map[string]bool{}
	var stack []string
	for name, root := range p.Roots {
		if _, ok := p.Paths[root]; !ok {
			return fmt.Errorf("root %s (%s) is not in the closure", name, root)
		}
		stack = append(stack, root)
	}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if reachable[cur] {
			continue
		}
		reachable[cur] = true
		for _, ref := range p.Paths[cur].References {
			if _, ok := p.Paths[ref]; !ok {
				return fmt.Errorf("%s references %s, which is not in the closure", cur, ref)
			}
			stack = append(stack, ref)
		}
	}
	var sorted []string
	for path := range p.Paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)
	for _, path := range sorted {
		info := p.Paths[path]
		if !storePathPattern.MatchString(path) {
			return fmt.Errorf("malformed store path %q", path)
		}
		if !reachable[path] {
			return fmt.Errorf("%s is not reachable from any root", path)
		}
		if !sriSHA256.MatchString(info.NarHash) || info.NarSize <= 0 {
			return fmt.Errorf("%s: malformed narHash or narSize", path)
		}
		for _, ref := range info.References {
			if !storePathPattern.MatchString(ref) {
				return fmt.Errorf("%s: malformed reference %q", path, ref)
			}
		}
		if err := VerifySignatures(path, info, publicKey); err != nil {
			return err
		}
	}
	return nil
}

// ClosureSize sums narSize over a platform's paths.
func (p PlatformLock) ClosureSize() int64 {
	var total int64
	for _, info := range p.Paths {
		total += info.NarSize
	}
	return total
}
