package nixbox

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// VerifyStore proves that the directory holds exactly the platform's closure:
// the same set of top-level entries, and for each its NAR hash and size equal
// the lock. Nix registers paths under the same hashes, so this holds for a
// store written by `nix copy` as well as for a copy of it.
func VerifyStore(p PlatformLock, storeDir string) error {
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, e := range entries {
		have[e.Name()] = true
	}
	var want []string
	for path := range p.Paths {
		want = append(want, path)
	}
	sort.Strings(want)
	wantSet := map[string]bool{}
	for _, path := range want {
		wantSet[filepath.Base(path)] = true
	}
	for name := range have {
		if !wantSet[name] {
			return fmt.Errorf("store holds %s, which the lock does not list", name)
		}
	}
	for _, path := range want {
		name := filepath.Base(path)
		if !have[name] {
			return fmt.Errorf("store lacks %s", name)
		}
		sum, size, err := NARHash(filepath.Join(storeDir, name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		info := p.Paths[path]
		if got := "sha256-" + base64.StdEncoding.EncodeToString(sum); got != info.NarHash || size != info.NarSize {
			return fmt.Errorf("%s: store content (%s, %d bytes) differs from the lock (%s, %d bytes)", name, got, size, info.NarHash, info.NarSize)
		}
	}
	return nil
}
