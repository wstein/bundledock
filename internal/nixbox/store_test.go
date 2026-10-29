package nixbox

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeStore creates a store directory with the two fixture paths and returns a
// platform lock that describes it exactly.
func fakeStore(t *testing.T) (string, PlatformLock) {
	t.Helper()
	return fakeStoreIn(t, t.TempDir())
}

func fakeStoreIn(t *testing.T, store string) (string, PlatformLock) {
	t.Helper()
	pl := PlatformLock{System: "x86_64-linux", Roots: map[string]string{"git": pathA}, Paths: map[string]PathInfo{}}
	for path, body := range map[string]string{pathA: "alpha", pathB: "beta"} {
		dir := filepath.Join(store, filepath.Base(path))
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", "tool"), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		sum, size, err := NARHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		pl.Paths[path] = PathInfo{NarHash: sri(sum), NarSize: size}
	}
	return store, pl
}

func TestVerifyStoreAcceptsAnExactStore(t *testing.T) {
	store, pl := fakeStore(t)
	if err := VerifyStore(pl, store); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyStoreRejects(t *testing.T) {
	cases := map[string]func(store string, pl PlatformLock){
		"modified file": func(s string, _ PlatformLock) {
			os.WriteFile(filepath.Join(s, filepath.Base(pathA), "bin", "tool"), []byte("ALPHA"), 0o755)
		},
		"mode change": func(s string, _ PlatformLock) {
			os.Chmod(filepath.Join(s, filepath.Base(pathA), "bin", "tool"), 0o644)
		},
		"added file": func(s string, _ PlatformLock) {
			os.WriteFile(filepath.Join(s, filepath.Base(pathB), "extra"), nil, 0o644)
		},
		"missing path": func(s string, _ PlatformLock) { os.RemoveAll(filepath.Join(s, filepath.Base(pathB))) },
		"extra path": func(s string, _ PlatformLock) {
			os.Mkdir(filepath.Join(s, "cccccccccccccccccccccccccccccccc-extra"), 0o755)
		},
		"stray file": func(s string, _ PlatformLock) { os.WriteFile(filepath.Join(s, "note"), nil, 0o644) },
		"size lies": func(_ string, pl PlatformLock) {
			i := pl.Paths[pathA]
			i.NarSize++
			pl.Paths[pathA] = i
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			store, pl := fakeStore(t)
			mutate(store, pl)
			if err := VerifyStore(pl, store); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
