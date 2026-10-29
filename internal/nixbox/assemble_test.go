package nixbox

import (
	"encoding/json"
	"strings"
	"testing"
)

func assembleInputs(t *testing.T, f *fixture) (map[string]map[string]string, map[string]map[string]PathInfo) {
	roots := map[string]map[string]string{}
	infos := map[string]map[string]PathInfo{}
	for arch, p := range f.lock.Platforms {
		roots[arch] = p.Roots
		infos[arch] = map[string]PathInfo{}
		for path, info := range p.Paths {
			infos[arch][path] = info
		}
		// A larger `nix path-info -r` result than the closure never hurts.
		extra := "/nix/store/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee-unrelated"
		infos[arch][extra] = sign(t, f.priv, extra, PathInfo{NarHash: p.Paths[pathA].NarHash, NarSize: 1})
	}
	return roots, infos
}

func TestAssembleKeepsTheClosureAndIsDeterministic(t *testing.T) {
	f := newFixture(t)
	roots, infos := assembleInputs(t, f)
	lock, err := Assemble(f.catalog, f.catalogData, roots, infos, f.key)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Platforms["arm64"].Paths) != 2 || lock.Platforms["arm64"].System != "aarch64-linux" {
		t.Fatalf("unexpected platform: %+v", lock.Platforms["arm64"])
	}
	a, _ := EncodeLock(lock)
	b, _ := EncodeLock(lock)
	if string(a) != string(b) || !strings.HasSuffix(string(a), "}\n") {
		t.Error("encoding is not stable")
	}
	var back Lock
	if err := json.Unmarshal(a, &back); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleRefusesUnsignedOrIncompleteInput(t *testing.T) {
	f := newFixture(t)
	roots, infos := assembleInputs(t, f)
	i := infos["arm64"][pathB]
	i.Signatures = nil
	infos["arm64"][pathB] = i
	if _, err := Assemble(f.catalog, f.catalogData, roots, infos, f.key); err == nil {
		t.Error("accepted an unsigned path")
	}
	roots, infos = assembleInputs(t, f)
	delete(infos["amd64"], pathB)
	if _, err := Assemble(f.catalog, f.catalogData, roots, infos, f.key); err == nil {
		t.Error("accepted a closure with a missing path")
	}
	roots, infos = assembleInputs(t, f)
	delete(roots, "arm64")
	if _, err := Assemble(f.catalog, f.catalogData, roots, infos, f.key); err == nil {
		t.Error("accepted a missing architecture")
	}
}
