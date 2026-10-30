package toolpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"bundledock/internal/nixbox"
)

const nixboxLockPath = "../../bundles/nixbox/packages.lock.json"

func TestNixboxIsSelectableButNotCanonical(t *testing.T) {
	b, err := SelectBundle("nixbox")
	if err != nil {
		t.Fatal(err)
	}
	want := Bundle{"nixbox", "bundles/nixbox", "publish-bundles.yml", "resolved-nixbox", "nixbox", "stable latest"}
	if b != want {
		t.Fatalf("%+v", b)
	}
	// `tools publish --all` and `tools upgrade --all` walk this list; nixbox is
	// published only when it is named.
	for _, name := range CanonicalBundles {
		if name == "nixbox" {
			t.Fatal("nixbox must not join --all")
		}
	}
	image, err := RepositoryImageForBundle("quay.io/bundledock", "nixbox")
	if err != nil || image != "quay.io/bundledock/nixbox" || BundleForMetadata(Metadata{Image: image}) != b {
		t.Fatalf("%s %v", image, err)
	}
}

func TestNixboxLockDigestPinsEveryStorePath(t *testing.T) {
	data, err := os.ReadFile(nixboxLockPath)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := LockDigest("nixbox", data)
	if err != nil || len(digest) != 64 {
		t.Fatalf("%s %v", digest, err)
	}
	if _, err = ToolDigest(data); err == nil {
		t.Fatal("a nixbox lock is not a tool lock")
	}
	var lock nixbox.Lock
	if err = json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	for path, info := range lock.Platforms["arm64"].Paths {
		info.NarHash = "sha256-" + strings.Repeat("A", 43) + "="
		lock.Platforms["arm64"].Paths[path] = info
		break
	}
	changed, _ := json.Marshal(lock)
	other, err := LockDigest("nixbox", changed)
	if err != nil || other == digest {
		t.Fatalf("a changed closure kept its digest: %s %v", other, err)
	}
	// The catalog digest alone is not a tool change.
	var fresh nixbox.Lock
	if err = json.Unmarshal(data, &fresh); err != nil {
		t.Fatal(err)
	}
	fresh.CatalogDigest = strings.Repeat("0", 64)
	same, _ := json.Marshal(fresh)
	if got, err := LockDigest("nixbox", same); err != nil || got != digest {
		t.Fatalf("%s %v", got, err)
	}
	toolbox, err := os.ReadFile("../../bundles/toolbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := ToolDigest(toolbox)
	if got, err := LockDigest("toolbox", toolbox); err != nil || got != want {
		t.Fatal("tool bundles keep their digest")
	}
	if _, err = LockDigest("nixbox", toolbox); err == nil {
		t.Fatal("a tool lock is not a nixbox lock")
	}
}

func TestNixboxBuildSmokesWithItsOwnScriptAndLabelsTheClosure(t *testing.T) {
	m, err := NewMetadataForBundle("nixbox", nixboxLockPath, "quay.io/example/nixbox", "2026.10.10-b1", strings.Repeat("f", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044", "1")
	if err != nil || m.RunAttempt != "1" {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err = NewMetadataForBundle("nixbox", nixboxLockPath, "quay.io/example/nixbox", "1.2.3-b1", strings.Repeat("f", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044", "1"); err == nil {
		t.Fatal("nixbox has no upstream version to tag")
	}
	f, r := buildFixture(t, m, nixboxLockPath, "amd64")
	if !r.Smoked {
		t.Fatal("not smoked")
	}
	smoked := false
	for _, c := range f.calls {
		smoked = smoked || reflect.DeepEqual(c, []string{"sh", "bundles/nixbox/scripts/smoke.sh", f.id, "consumer:test"})
	}
	if !smoked {
		t.Fatalf("%v", f.calls)
	}
	data, _ := os.ReadFile(nixboxLockPath)
	digest, _ := LockDigestFile("nixbox", nixboxLockPath)
	if f.labels[platformsLabel] != digest || f.labels["io.bundledock.bundle"] != "nixbox" || f.labels["org.opencontainers.image.title"] != "bundledock-nixbox" {
		t.Fatalf("%v", f.labels)
	}
	var roots map[string]string
	if err = json.Unmarshal([]byte(f.labels["io.bundledock.tools.versions"]), &roots); err != nil {
		t.Fatal(err)
	}
	lock, err := nixbox.ParseLock(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != len(lock.Platforms["amd64"].Roots)+8 || !strings.HasSuffix(roots["git"], "-git-minimal-2.56.0") || strings.Contains(roots["git"], "/") {
		t.Fatalf("%v", roots)
	}
	if _, err = f.adapter().Push(m, nixboxLockPath, r); err != nil {
		t.Fatal(err)
	}
	var staged bool
	for _, c := range f.calls {
		staged = staged || reflect.DeepEqual(c, []string{"docker", "tag", f.id, "quay.io/example/nixbox:staging-2026.10.10-b1-amd64"})
	}
	if !staged {
		t.Fatalf("%v", f.calls)
	}
}

func TestNixboxUsesTheDailyTagNamespace(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"token":"token"}`
		if calls > 1 {
			if r.URL.Path != "/v2/example/nixbox/tags/list" {
				t.Fatal(r.URL.Path)
			}
			body = `{"name":"example/nixbox","tags":["2026.10.15-b2","staging-2026.10.15-b3-amd64"]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(body))), Header: http.Header{}}, nil
	})}
	got, err := AllocateToolboxTag(context.Background(), client, "quay.io/example/nixbox", "2026.10.15", "robot", "secret")
	if err != nil || got != "2026.10.15-b4" {
		t.Fatalf("%s %v", got, err)
	}
	if _, err = AllocateToolboxTag(context.Background(), client, "quay.io/example/node", "2026.10.15", "robot", "secret"); err == nil {
		t.Fatal("daily tags belong to toolbox and nixbox only")
	}
	if _, err = SingletonVersion([]byte("{}"), "nixbox"); err == nil {
		t.Fatal("nixbox is not a singleton bundle")
	}
}

// The publication record of nixbox covers both of its locks: the Nix store paths and
// the official release binaries in bundles/nixbox/tools.
func nixboxCopy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"packages.lock.json", "tools/packages.lock.json"} {
		data, err := os.ReadFile("../../bundles/nixbox/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(dir+"/"+strings.TrimSuffix(name, "packages.lock.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(dir+"/"+name, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir + "/packages.lock.json"
}

func TestNixboxRecordCoversTheToolsLock(t *testing.T) {
	lock := nixboxCopy(t)
	tools := ToolsLockPath(lock)
	digest, err := LockDigestFile("nixbox", lock)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := LockSHA256("nixbox", lock)
	if err != nil {
		t.Fatal(err)
	}
	nixOnly, _ := os.ReadFile(lock)
	if single, _ := LockDigest("nixbox", nixOnly); single == digest {
		t.Error("the nixbox digest ignores the tools lock")
	}
	if plain, _ := HashFile(lock); plain == sum {
		t.Error("the nixbox lock hash ignores the tools lock")
	}
	data, _ := os.ReadFile(tools)
	if err = os.WriteFile(tools, []byte(strings.Replace(string(data), `"version": "v`, `"version": "v9`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	changedDigest, _ := LockDigestFile("nixbox", lock)
	changedSum, _ := LockSHA256("nixbox", lock)
	if changedDigest == digest || changedSum == sum {
		t.Error("a tools lock change left the nixbox digest or lock hash unchanged")
	}
	if err = os.Remove(tools); err != nil {
		t.Fatal(err)
	}
	if _, err = LockDigestFile("nixbox", lock); err == nil {
		t.Error("digest without a tools lock")
	}
}

func TestOtherBundlesKeepTheirDigestAndHash(t *testing.T) {
	lock := "../../bundles/toolbox/packages.lock.json"
	data, _ := os.ReadFile(lock)
	want, _ := ToolDigest(data)
	if got, err := LockDigestFile("toolbox", lock); err != nil || got != want {
		t.Fatalf("%s %v", got, err)
	}
	plain, _ := HashFile(lock)
	if got, err := LockSHA256("toolbox", lock); err != nil || got != plain {
		t.Fatalf("%s %v", got, err)
	}
}

func TestNixboxVersionsLabelListsTheOfficialBinaries(t *testing.T) {
	lock := nixboxCopy(t)
	m := Metadata{Bundle: "nixbox", Source: "https://github.com/example/repo", Commit: strings.Repeat("a", 40), Created: "2026-10-10T03:17:00Z", Tag: "2026.10.10-b1", RunID: "1"}
	got, err := labels(m, lock, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	var versions map[string]string
	if err = json.Unmarshal([]byte(got["io.bundledock.tools.versions"]), &versions); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"zk", "helix", "difftastic", "ast-grep", "git"} {
		if versions[tool] == "" {
			t.Errorf("no version for %s: %v", tool, versions)
		}
	}
	if strings.Contains(versions["zk"], "/") || !strings.HasPrefix(versions["zk"], "v") {
		t.Errorf("zk version %q", versions["zk"])
	}
	if want, _ := LockDigestFile("nixbox", lock); got[platformsLabel] != want {
		t.Error("platforms label differs from the file digest")
	}
}
