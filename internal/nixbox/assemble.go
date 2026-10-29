package nixbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Assemble builds a lock from `nix eval` roots and `nix path-info` results for
// every architecture. Only paths reachable from the roots are kept, and the
// result must pass CheckLock, so an unsigned or incomplete input never yields a
// lock.
func Assemble(c *Catalog, catalogData []byte, roots map[string]map[string]string, infos map[string]map[string]PathInfo, publicKey string) (*Lock, error) {
	lock := &Lock{Schema: 1, CatalogDigest: CatalogDigest(catalogData), Platforms: map[string]PlatformLock{}}
	for arch, system := range c.Platforms {
		r, ok := roots[arch]
		if !ok {
			return nil, fmt.Errorf("no roots for %s", arch)
		}
		all, ok := infos[arch]
		if !ok {
			return nil, fmt.Errorf("no path info for %s", arch)
		}
		kept := map[string]PathInfo{}
		var stack []string
		for _, root := range r {
			stack = append(stack, root)
		}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, done := kept[cur]; done {
				continue
			}
			info, ok := all[cur]
			if !ok {
				return nil, fmt.Errorf("%s: no path info for %s", arch, cur)
			}
			info.References = append([]string{}, info.References...)
			sort.Strings(info.References)
			info.Signatures = append([]string{}, info.Signatures...)
			kept[cur] = info
			stack = append(stack, info.References...)
		}
		lock.Platforms[arch] = PlatformLock{System: system, Roots: r, Paths: kept}
	}
	if err := CheckLock(c, catalogData, lock, publicKey); err != nil {
		return nil, err
	}
	return lock, nil
}

// EncodeLock writes the canonical lock form: sorted keys, two-space indent,
// one trailing newline.
func EncodeLock(l *Lock) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(l); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ReadLockInputs reads roots-<arch>.json (package name to store path, from
// nix eval) and pathinfo-<arch>.json (nix path-info --json --json-format 1 -r)
// for every catalog architecture from dir.
func ReadLockInputs(dir string, c *Catalog) (map[string]map[string]string, map[string]map[string]PathInfo, error) {
	roots := map[string]map[string]string{}
	infos := map[string]map[string]PathInfo{}
	for arch := range c.Platforms {
		var r map[string]string
		if err := readJSON(filepath.Join(dir, "roots-"+arch+".json"), &r); err != nil {
			return nil, nil, err
		}
		var i map[string]PathInfo
		if err := readJSON(filepath.Join(dir, "pathinfo-"+arch+".json"), &i); err != nil {
			return nil, nil, err
		}
		roots[arch], infos[arch] = r, i
	}
	return roots, infos, nil
}

// readJSON decodes one JSON document; path-info records carry extra fields the
// lock does not keep, so unknown fields are accepted here (the lock itself is strict).
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
