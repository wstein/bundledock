package nixbox

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const (
	pathA = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-git-minimal-2.0"
	pathB = "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-glibc-2.0"
)

type fixture struct {
	catalogData []byte
	catalog     *Catalog
	lock        *Lock
	key         string
	priv        ed25519.PrivateKey
}

func sign(t *testing.T, priv ed25519.PrivateKey, path string, info PathInfo) PathInfo {
	t.Helper()
	fp, err := fingerprint(path, info)
	if err != nil {
		t.Fatal(err)
	}
	info.Signatures = []string{"cache.test-1:" + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(fp)))}
	return info
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(nil)
	data := []byte(validCatalog)
	c, err := ParseCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	hash := "sha256-yOXkKK/xzGw2X2QPhEgJcVlERPLZeHOIz6X6nti1h2A="
	platform := func(system string) PlatformLock {
		return PlatformLock{System: system, Roots: map[string]string{"git": pathA}, Paths: map[string]PathInfo{
			pathA: sign(t, priv, pathA, PathInfo{NarHash: hash, NarSize: 10, References: []string{pathA, pathB}}),
			pathB: sign(t, priv, pathB, PathInfo{NarHash: hash, NarSize: 20, References: []string{pathB}}),
		}}
	}
	return &fixture{data, c, &Lock{Schema: 1, CatalogDigest: CatalogDigest(data), Platforms: map[string]PlatformLock{
		"amd64": platform("x86_64-linux"), "arm64": platform("aarch64-linux")}},
		"cache.test-1:" + base64.StdEncoding.EncodeToString(pub), priv}
}

func (f *fixture) check() error { return CheckLock(f.catalog, f.catalogData, f.lock, f.key) }

func TestLockAcceptsASignedClosure(t *testing.T) {
	f := newFixture(t)
	if err := f.check(); err != nil {
		t.Fatal(err)
	}
	if got := f.lock.Platforms["arm64"].ClosureSize(); got != 30 {
		t.Errorf("closure size %d", got)
	}
}

func TestLockRejects(t *testing.T) {
	mutations := map[string]func(*fixture){
		"stale catalog":    func(f *fixture) { f.lock.CatalogDigest = strings.Repeat("0", 64) },
		"schema":           func(f *fixture) { f.lock.Schema = 2 },
		"missing platform": func(f *fixture) { delete(f.lock.Platforms, "arm64") },
		"wrong system": func(f *fixture) {
			p := f.lock.Platforms["arm64"]
			p.System = "x86_64-linux"
			f.lock.Platforms["arm64"] = p
		},
		"missing root": func(f *fixture) { f.lock.Platforms["arm64"].Roots["extra"] = pathA },
		"root not in paths": func(f *fixture) {
			f.lock.Platforms["arm64"].Roots["git"] = "/nix/store/cccccccccccccccccccccccccccccccc-x"
		},
		"dangling reference": func(f *fixture) {
			p := f.lock.Platforms["arm64"]
			i := p.Paths[pathB]
			i.References = append(i.References, "/nix/store/cccccccccccccccccccccccccccccccc-x")
			p.Paths[pathB] = sign(t, f.priv, pathB, i)
		},
		"unreachable path": func(f *fixture) {
			p := f.lock.Platforms["arm64"]
			extra := "/nix/store/dddddddddddddddddddddddddddddddd-extra"
			p.Paths[extra] = sign(t, f.priv, extra, PathInfo{NarHash: p.Paths[pathA].NarHash, NarSize: 1})
		},
		"bad path name": func(f *fixture) {
			p := f.lock.Platforms["arm64"]
			p.Paths["/nix/store/short-x"] = p.Paths[pathB]
			p.Roots["git"] = "/nix/store/short-x"
		},
		"narSize changed": func(f *fixture) {
			p := f.lock.Platforms["arm64"]
			i := p.Paths[pathB]
			i.NarSize++
			p.Paths[pathB] = i
		},
		"unsigned": func(f *fixture) {
			p := f.lock.Platforms["arm64"]
			i := p.Paths[pathB]
			i.Signatures = nil
			p.Paths[pathB] = i
		},
		"signed by another key": func(f *fixture) {
			_, other, _ := ed25519.GenerateKey(nil)
			p := f.lock.Platforms["arm64"]
			p.Paths[pathB] = sign(t, other, pathB, p.Paths[pathB])
		},
		"bad hash": func(f *fixture) {
			p := f.lock.Platforms["arm64"]
			i := p.Paths[pathB]
			i.NarHash = "sha256-short"
			p.Paths[pathB] = i
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			mutate(f)
			if err := f.check(); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestParseLockIsStrict(t *testing.T) {
	f := newFixture(t)
	data, _ := json.Marshal(f.lock)
	if _, err := ParseLock(data); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLock([]byte(strings.Replace(string(data), `"schema":1`, `"schema":1,"extra":1`, 1))); err == nil {
		t.Error("accepted an unknown field")
	}
	if _, err := ParseLock(append(data, '{')); err == nil {
		t.Error("accepted trailing data")
	}
}

// The production lock must satisfy the catalog and the pinned cache key. It
// asserts properties, not versions, so a lock refresh needs no test change.
func TestProductionLockVerifies(t *testing.T) {
	data, c := productionCatalog(t)
	raw, err := os.ReadFile("../../bundles/nixbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	l, err := ParseLock(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckLock(c, data, l, CachePublicKey); err != nil {
		t.Fatal(err)
	}
}
