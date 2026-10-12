package toolmanager

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = f.base.Scheme
	u.Host = f.base.Host
	clone.URL = &u
	return f.transport.RoundTrip(clone)
}
func fixtureClient(t *testing.T, h http.HandlerFunc) *http.Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	u, _ := url.Parse(s.URL)
	return &http.Client{Transport: fixtureTransport{u, s.Client().Transport}}
}
func checkout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"packages.txt", "packages.catalog.json", "packages.lock.json"} {
		b, err := os.ReadFile(filepath.Join("../../bundles/toolbox", n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func readLock(t *testing.T, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "packages.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func releases(t *testing.T, assets []map[string]string) []byte {
	t.Helper()
	b, err := json.Marshal([]any{map[string]any{"tag_name": "v2.103.0", "draft": false, "prerelease": false, "assets": assets}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestSingleUpgradeBothArchitecturesAndAtomicFailure(t *testing.T) {
	payload := []byte("verified binary")
	sum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	for _, failure := range []string{"", "missing-arm64", "checksum", "http", "output"} {
		t.Run(failure, func(t *testing.T) {
			dir := checkout(t)
			before := readLock(t, dir)
			old, _ := toolconfig.Load(dir)
			oldCatalog := old.CatalogDigest()
			assets := []map[string]string{{"name": "gh_2.103.0_linux_amd64.tar.gz", "browser_download_url": "https://github.com/cli/cli/releases/download/v2.103.0/gh_2.103.0_linux_amd64.tar.gz", "digest": sum}}
			if failure != "missing-arm64" {
				assets = append(assets, map[string]string{"name": "gh_2.103.0_linux_arm64.tar.gz", "browser_download_url": "https://github.com/cli/cli/releases/download/v2.103.0/gh_2.103.0_linux_arm64.tar.gz", "digest": sum})
			}
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/repos/") {
					w.Write(releases(t, assets))
					return
				}
				if failure == "http" {
					http.Error(w, "failure", 503)
					return
				}
				if failure == "checksum" {
					w.Write([]byte("changed"))
					return
				}
				w.Write(payload)
			})
			var output io.Writer = &bytes.Buffer{}
			if failure == "output" {
				output = failedWriter{}
			}
			err := Upgrade(context.Background(), client, dir, "gh", output)
			if failure != "" {
				if err == nil {
					t.Fatal("expected failure")
				}
				if !bytes.Equal(before, readLock(t, dir)) {
					t.Fatal("failure changed lock bytes")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			c, err := toolconfig.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if c.CatalogDigest() != oldCatalog || len(c.Catalog.Packages["codex"].Install.ExcludeDirectories) != 1 || !c.Catalog.Packages["codex"].Install.Excludes("codex-resources/voice/lib.so") {
				t.Fatal("targeted upgrade changed Codex exclusion recipe or its identity")
			}
			for _, a := range []string{"amd64", "arm64"} {
				if c.Lock.Platforms[a]["gh"].Version != "v2.103.0" {
					t.Fatal("not upgraded")
				}
				for n, s := range old.Lock.Platforms[a] {
					if n != "gh" && c.Lock.Platforms[a][n] != s {
						t.Fatalf("changed %s", n)
					}
				}
			}
		})
	}
}

func TestAllProvidersPaginationRangesAndVerification(t *testing.T) {
	dir := checkout(t)
	c, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("official fixture")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))
	versions := map[string]string{}
	repos := map[string]string{}
	for n, p := range c.Catalog.Packages {
		v := c.Desired[n].Lower
		if !c.Desired[n].Exact {
			v[2]++
		}
		versions[n] = v.String()
		repos[p.Repository] = n
	}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases")
			n := repos[repo]
			p := c.Catalog.Packages[n]
			v := fixtureReleaseVersion(p, versions[n])
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", `<https://api.github.com`+r.URL.Path+`?per_page=20&page=2>; rel="next"`)
				json.NewEncoder(w).Encode([]any{map[string]any{"tag_name": p.TagPrefix + "0.0.0"}, map[string]any{"tag_name": p.TagPrefix + v + "-rc.1"}, map[string]any{"tag_name": p.TagPrefix + v, "draft": true}})
				return
			}
			assets := []asset{}
			for _, arch := range []string{"amd64", "arm64"} {
				name := expand(p.Asset, v, p.Architectures[arch])
				a := asset{Name: name, URL: "https://github.com/" + p.Repository + "/releases/download/" + p.TagPrefix + v + "/" + name, Digest: "sha256:" + sum}
				if n == "uv" || n == "starship" {
					a.Digest = ""
					assets = append(assets, asset{Name: name + ".sha256", URL: a.URL + ".sha256"})
				}
				if n == "ast-grep" {
					a.Digest = ""
				}
				assets = append(assets, a)
			}
			json.NewEncoder(w).Encode([]release{{Tag: p.TagPrefix + v, Assets: assets}, {Tag: p.TagPrefix + c.Desired[n].Lower.String()}})
			return
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			json.NewEncoder(w).Encode(map[string]string{"version": "1.3.2"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/manifest.json") {
			json.NewEncoder(w).Encode(map[string]any{"version": versions["claude"], "platforms": map[string]any{"linux-x64": map[string]string{"checksum": sum}, "linux-arm64": map[string]string{"checksum": sum}}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/latest") {
			fmt.Fprint(w, versions["claude"])
			return
		}
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			fmt.Fprintln(w, sum)
			return
		}
		w.Write(payload)
	})
	if err := Upgrade(context.Background(), client, dir, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	updated, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for n, p := range c.Catalog.Packages {
		for _, arch := range []string{"amd64", "arm64"} {
			s := updated.Lock.Platforms[arch][n]
			v, _ := p.Version(s.Version)
			if v != versions[n] || s.SHA256 != sum {
				t.Fatalf("%s/%s: %+v", arch, n, s)
			}
			want := "publisher-sha256"
			if n == "ast-grep" {
				want = "tls-download-sha256"
			}
			if s.Verification != want {
				t.Fatalf("%s provenance %s", n, s.Verification)
			}
		}
	}
}
func TestLatestMissingArchitectureDoesNotFallBack(t *testing.T) {
	dir := checkout(t)
	before := readLock(t, dir)
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]release{{Tag: "v2.104.0"}, {Tag: "v2.103.0", Assets: []asset{{Name: "gh_2.103.0_linux_amd64.tar.gz", URL: "https://github.com/a"}, {Name: "gh_2.103.0_linux_arm64.tar.gz", URL: "https://github.com/b"}}}})
	})
	if err := Upgrade(context.Background(), client, dir, "gh", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "2.104.0") {
		t.Fatalf("unexpected error %v", err)
	}
	if !bytes.Equal(before, readLock(t, dir)) {
		t.Fatal("changed lock")
	}
}
func TestAntigravityCurrentManifestWinsOverHistoricalGitHub(t *testing.T) {
	dir := checkout(t)
	payload := []byte("current agy")
	sum512 := fmt.Sprintf("%x", sha512.Sum512(payload))

	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			json.NewEncoder(w).Encode([]release{{Tag: "1.3.2"}})
			return
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			platform := "linux-x64/cli_linux_x64.tar.gz"
			if strings.Contains(r.URL.Path, "arm64") {
				platform = "linux-arm/cli_linux_arm64.tar.gz"
			}
			json.NewEncoder(w).Encode(map[string]string{"version": "1.3.3", "url": "https://storage.googleapis.com/antigravity-public/antigravity-cli/1.3.3-1234/" + platform, "sha512": sum512})
			return
		}
		w.Write(payload)
	})
	if err := Upgrade(context.Background(), client, dir, "agy", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	c, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		s := c.Lock.Platforms[arch]["agy"]
		if s.Version != "1.3.3" || s.SHA512 != sum512 || s.Verification != "publisher-sha512" {
			t.Fatalf("%+v", s)
		}
	}
}

func TestChangedDesiredAndBootstrap(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		t.Run(fmt.Sprint(bootstrap), func(t *testing.T) {
			dir := checkout(t)
			c, _ := toolconfig.Load(dir)
			if bootstrap {
				if err := os.Remove(filepath.Join(dir, "packages.lock.json")); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(filepath.Join(dir, "packages.txt"))
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.ReplaceAll(data, []byte("gh>=2.102.0"), []byte("gh==3.0.0"))
				if err := os.WriteFile(filepath.Join(dir, "packages.txt"), data, 0644); err != nil {
					t.Fatal(err)
				}
				if _, err := toolconfig.Load(dir); err == nil {
					t.Fatal("builder accepted stale identity")
				}
			}
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/repos/") {
					repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases")
					for n, p := range c.Catalog.Packages {
						if p.Repository == repo {
							v := fixtureReleaseVersion(p, c.Desired[n].Lower.String())
							if !bootstrap && n == "gh" {
								v = "3.0.0"
							}
							assets := []asset{}
							for _, arch := range []string{"amd64", "arm64"} {
								name := expand(p.Asset, v, p.Architectures[arch])
								assets = append(assets, asset{Name: name, URL: "https://github.com/" + p.Repository + "/releases/download/" + p.TagPrefix + v + "/" + name})
							}
							json.NewEncoder(w).Encode([]release{{Tag: p.TagPrefix + v, Assets: assets}})
							return
						}
					}
				}
				if strings.Contains(r.URL.Path, "/manifests/") {
					json.NewEncoder(w).Encode(map[string]string{"version": "1.3.2"})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/latest") {
					fmt.Fprint(w, "2.1.296")
					return
				}
				if strings.HasSuffix(r.URL.Path, "manifest.json") {
					sum := fmt.Sprintf("%x", sha256.Sum256([]byte("binary")))
					fmt.Fprintf(w, `{"platforms":{"linux-x64":{"checksum":"%s"},"linux-arm64":{"checksum":"%s"}}}`, sum, sum)
					return
				}
				fmt.Fprint(w, "binary")
			})
			target := "gh"
			if bootstrap {
				target = ""
			}
			if err := Upgrade(context.Background(), client, dir, target, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if _, err := toolconfig.Load(dir); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClaudeCurrentReleaseIsDiscovered(t *testing.T) {
	dir := checkout(t)
	payload := []byte("current claude")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			json.NewEncoder(w).Encode([]release{{Tag: "v2.1.296"}})
		case strings.HasSuffix(r.URL.Path, "/latest"):
			fmt.Fprint(w, "2.1.297\n")
		case strings.HasSuffix(r.URL.Path, "/manifest.json"):
			fmt.Fprintf(w, `{"version":"2.1.297","platforms":{"linux-x64":{"checksum":"%s"},"linux-arm64":{"checksum":"%s"}}}`, sum, sum)
		default:
			w.Write(payload)
		}
	})
	if err := Upgrade(context.Background(), client, dir, "claude", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	c, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Lock.Platforms["arm64"]["claude"].Version != "2.1.297" {
		t.Fatal("current release omitted")
	}
}
func TestMetadataErrorsLeaveLockUnchanged(t *testing.T) {
	for _, kind := range []string{"malformed", "truncated", "pagination", "no-match", "prerelease", "invalid-digest", "redirect"} {
		t.Run(kind, func(t *testing.T) {
			dir := checkout(t)
			before := readLock(t, dir)
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "malformed":
					fmt.Fprint(w, "{")
				case "truncated":
					w.Header().Set("Content-Length", "99")
					fmt.Fprint(w, "[]")
				case "pagination":
					w.Header().Set("Link", `<https://evil.example/releases>; rel="next"`)
					fmt.Fprint(w, "[]")
				case "no-match":
					fmt.Fprint(w, `[{"tag_name":"v999.0.0"}]`)
				case "prerelease":
					fmt.Fprint(w, `[{"tag_name":"v2.103.0","prerelease":true},{"tag_name":"v2.104.0-rc.1"}]`)
				case "invalid-digest":
					fmt.Fprint(w, `[{"tag_name":"v2.103.0","assets":[{"name":"gh_2.103.0_linux_amd64.tar.gz","browser_download_url":"https://github.com/asset","digest":"sha256:no"}]}]`)
				case "redirect":
					http.Redirect(w, r, "http://evil.example/artifact", http.StatusFound)
				}
			})
			if err := Upgrade(context.Background(), client, dir, "gh", &bytes.Buffer{}); err == nil {
				t.Fatal("accepted invalid upstream metadata")
			}
			if !bytes.Equal(before, readLock(t, dir)) {
				t.Fatal("failure changed lock bytes")
			}
		})
	}
}

func TestEmptyPublishedDigestFailsClosed(t *testing.T) {
	dir := checkout(t)
	before := readLock(t, dir)
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			json.NewEncoder(w).Encode([]release{{Tag: "v2.103.0", Assets: []asset{{Name: "gh_2.103.0_linux_amd64.tar.gz", URL: "https://github.com/amd64", Digest: "sha256:"}, {Name: "gh_2.103.0_linux_arm64.tar.gz", URL: "https://github.com/arm64", Digest: "sha256:"}}}})
			return
		}
		fmt.Fprint(w, "binary")
	})
	if err := Upgrade(context.Background(), client, dir, "gh", &bytes.Buffer{}); err == nil {
		t.Fatal("accepted present empty publisher checksum")
	}
	if !bytes.Equal(before, readLock(t, dir)) {
		t.Fatal("changed lock")
	}
}
func TestAntigravityCurrentManifestSameVersion(t *testing.T) {
	dir := checkout(t)
	payload := []byte("current agy")
	sum512 := fmt.Sprintf("%x", sha512.Sum512(payload))
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			json.NewEncoder(w).Encode([]release{{Tag: "1.3.2"}})
			return
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			json.NewEncoder(w).Encode(map[string]string{"version": "1.3.2", "url": "https://storage.googleapis.com/antigravity-public/current.tar.gz", "sha512": sum512})
			return
		}
		w.Write(payload)
	})
	if err := Upgrade(context.Background(), client, dir, "agy", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
func TestUpgradeAllRepairsChangedCatalogInventory(t *testing.T) {
	dir := checkout(t)
	c, _ := toolconfig.Load(dir)
	delete(c.Catalog.Packages, "claude")
	delete(c.Desired, "claude")
	added := c.Catalog.Packages["gh"]
	added.Repository = "example/new-tool"
	c.Catalog.Packages["new-tool"] = added
	c.Desired["new-tool"] = c.Desired["gh"]
	catalog, err := json.Marshal(c.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packages.catalog.json"), catalog, 0644); err != nil {
		t.Fatal(err)
	}
	var desired strings.Builder
	for n, v := range c.Desired {
		fmt.Fprintln(&desired, n+v.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "packages.txt"), []byte(desired.String()), 0644); err != nil {
		t.Fatal(err)
	}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases")
			for n, p := range c.Catalog.Packages {
				if p.Repository == repo {
					v := fixtureReleaseVersion(p, c.Desired[n].Lower.String())
					assets := []asset{}
					for _, arch := range []string{"amd64", "arm64"} {
						name := expand(p.Asset, v, p.Architectures[arch])
						assets = append(assets, asset{Name: name, URL: "https://github.com/" + p.Repository + "/releases/download/" + p.TagPrefix + v + "/" + name})
					}
					json.NewEncoder(w).Encode([]release{{Tag: p.TagPrefix + v, Assets: assets}})
					return
				}
			}
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			json.NewEncoder(w).Encode(map[string]string{"version": "1.3.1"})
			return
		}
		fmt.Fprint(w, "binary")
	})
	if err := Upgrade(context.Background(), client, dir, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	updated, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.Lock.Platforms["arm64"]["new-tool"]; !ok {
		t.Fatal("added tool missing")
	}
	if _, ok := updated.Lock.Platforms["amd64"]["claude"]; ok {
		t.Fatal("removed tool retained")
	}
}
func TestUpstreamRedirectPolicy(t *testing.T) {
	for _, target := range []string{"http://github.com/payload", "https://evil.example/payload"} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; http.Redirect(w, r, target, http.StatusFound) })
			if _, err := (resolver{client: client}).get(context.Background(), "https://github.com/artifact"); err == nil {
				t.Fatal("unsafe redirect accepted")
			}
			if calls != 1 {
				t.Fatalf("followed unsafe redirect: %d requests", calls)
			}
		})
	}
}

func TestTargetedUpgradeRequiresUnchangedCatalog(t *testing.T) {
	dir := checkout(t)
	c, _ := toolconfig.Load(dir)
	before := readLock(t, dir)
	p := c.Catalog.Packages["gh"]
	p.Sidecar = "changed-checksums.txt"
	c.Catalog.Packages["gh"] = p
	data, _ := json.Marshal(c.Catalog)
	if err := os.WriteFile(filepath.Join(dir, "packages.catalog.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("requested metadata before rejecting changed catalog")
	})
	if err := Upgrade(context.Background(), client, dir, "codex", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "without a tool") {
		t.Fatalf("unexpected error %v", err)
	}
	if !bytes.Equal(before, readLock(t, dir)) {
		t.Fatal("changed lock")
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("output unavailable") }

func TestGitHubAuthenticationIsScopedToAPIHost(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fixture-secret")
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		want := ""
		if r.URL.Path == "/api-fixture" {
			want = "Bearer fixture-secret"
		}
		if r.Header.Get("Authorization") != want {
			t.Errorf("incorrect authentication scope for %s", r.URL.Path)
		}
		fmt.Fprint(w, "[]")
	})
	for _, raw := range []string{"https://api.github.com/api-fixture", "https://github.com/artifact", "https://downloads.claude.ai/claude", "https://storage.googleapis.com/antigravity", "https://downloads.claude.ai/other-artifact"} {
		resp, err := (resolver{client: client}).get(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
}

func TestNullGitHubDigestMeansUnpublished(t *testing.T) {
	for _, digest := range []string{"null", `""`} {
		t.Run(digest, func(t *testing.T) {
			dir := checkout(t)
			before := readLock(t, dir)
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/repos/") {
					fmt.Fprintf(w, `[{"tag_name":"v2.103.0","assets":[{"name":"gh_2.103.0_linux_amd64.tar.gz","browser_download_url":"https://github.com/amd64","digest":%s},{"name":"gh_2.103.0_linux_arm64.tar.gz","browser_download_url":"https://github.com/arm64","digest":%s}]}]`, digest, digest)
					return
				}
				fmt.Fprint(w, "historical binary")
			})
			err := Upgrade(context.Background(), client, dir, "gh", &bytes.Buffer{})
			if digest != "null" {
				if err == nil {
					t.Fatal("accepted empty string publisher checksum")
				}
				if !bytes.Equal(before, readLock(t, dir)) {
					t.Fatal("failure changed lock")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			c, err := toolconfig.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, arch := range []string{"amd64", "arm64"} {
				if c.Lock.Platforms[arch]["gh"].Verification != "tls-download-sha256" {
					t.Fatal("null digest misrepresented as published")
				}
			}
		})
	}
}

// Fixture release tags retain tmux's native two-component/letter spelling.
func fixtureReleaseVersion(p toolconfig.Package, canonical string) string {
	if p.Normalization == "helix-calendar" {
		v, _ := toolconfig.ParseVersion(canonical)
		return fmt.Sprintf("%d.%02d.%d", v[0], v[1], v[2])
	}
	if p.Normalization != "tmux-release" {
		return canonical
	}
	v, _ := toolconfig.ParseVersion(canonical)
	if v[2] == 0 {
		return fmt.Sprintf("%d.%d", v[0], v[1])
	}
	return fmt.Sprintf("%d.%d%c", v[0], v[1], 'a'+v[2]-1)
}
