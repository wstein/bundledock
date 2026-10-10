package toolmanager

import (
	"anchorage/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/json"
	"fmt"
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
	for _, n := range []string{"tools.txt", "tools.packages.json", "tools.lock.json"} {
		b, err := os.ReadFile(filepath.Join("../..", n))
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
	b, err := os.ReadFile(filepath.Join(dir, "tools.lock.json"))
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
	for _, failure := range []string{"", "missing-arm64", "checksum", "http"} {
		t.Run(failure, func(t *testing.T) {
			dir := checkout(t)
			before := readLock(t, dir)
			old, _ := toolconfig.Load(dir)
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
			err := Upgrade(context.Background(), client, dir, "gh", &bytes.Buffer{})
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
	sha3sum := fmt.Sprintf("%x", sha3.Sum256(payload))
	versions := map[string]string{}
	repos := map[string]string{}
	for n, p := range c.Catalog.Packages {
		v := c.Desired[n].Lower
		v[2]++
		versions[n] = v.String()
		repos[p.Repository] = n
	}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases")
			n := repos[repo]
			p := c.Catalog.Packages[n]
			v := versions[n]
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", `<https://api.github.com`+r.URL.Path+`?per_page=100&page=2>; rel="next"`)
				json.NewEncoder(w).Encode([]any{map[string]any{"tag_name": p.TagPrefix + "999.0.0"}, map[string]any{"tag_name": p.TagPrefix + v + "-rc.1"}, map[string]any{"tag_name": p.TagPrefix + v, "draft": true}})
				return
			}
			assets := []asset{}
			for _, arch := range []string{"amd64", "arm64"} {
				name := expand(p.Asset, v, p.Architectures[arch], "", "")
				a := asset{Name: name, URL: "https://github.com/" + p.Repository + "/releases/download/" + p.TagPrefix + v + "/" + name, Digest: "sha256:" + sum}
				if n == "uv" {
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
		if strings.HasSuffix(r.URL.Path, "/chronology.html") {
			fmt.Fprintf(w, `<table id="chrontab"><tr><td>2026-10-09</td><td><a href="releaselog/3_54_1.html">%s</a></td></tr><tr><td>2027-01-01</td><td><a href="releaselog/3_51_3.html">3.51.3</a></td></tr></table>`, versions["sqlite3"])
			return
		}
		if strings.HasSuffix(r.URL.Path, "/download.html") {
			compact, _ := SQLiteCompact(versions["sqlite3"])
			fmt.Fprintf(w, "PRODUCT,%s,2026/sqlite-amalgamation-%s.zip,16,%s\n", versions["sqlite3"], compact, sha3sum)
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
			if n == "sqlite3" {
				want = "publisher-sha3-256"
			}
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
func TestSQLiteHistoricalAndPublisherMismatch(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprint(mismatch), func(t *testing.T) {
			dir := checkout(t)
			before := readLock(t, dir)
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "chronology.html"):
					fmt.Fprint(w, `<tr><td>2026-10-09</td><td><a href="releaselog/3_54_0.html">3.54.0</a></td></tr>`)
				case strings.HasSuffix(r.URL.Path, "download.html"):
					if mismatch {
						fmt.Fprintf(w, "PRODUCT,3.54.0,2026/sqlite-amalgamation-3540000.zip,10,%s\n", strings.Repeat("a", 64))
					}
				default:
					fmt.Fprint(w, "historical official bytes")
				}
			})
			err := Upgrade(context.Background(), client, dir, "sqlite3", &bytes.Buffer{})
			if mismatch {
				if err == nil || !bytes.Equal(before, readLock(t, dir)) {
					t.Fatal("accepted mismatch or changed lock")
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
			if c.Lock.Platforms["amd64"]["sqlite3"].Verification != "tls-download-sha256" {
				t.Fatal("historical hash misrepresented")
			}
		})
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
				if err := os.Remove(filepath.Join(dir, "tools.lock.json")); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(filepath.Join(dir, "tools.txt"))
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.ReplaceAll(data, []byte("gh>=2.102.0,<3.0.0"), []byte("gh==3.0.0"))
				if err := os.WriteFile(filepath.Join(dir, "tools.txt"), data, 0644); err != nil {
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
							v := c.Desired[n].Lower.String()
							if !bootstrap && n == "gh" {
								v = "3.0.0"
							}
							assets := []asset{}
							for _, arch := range []string{"amd64", "arm64"} {
								name := expand(p.Asset, v, p.Architectures[arch], "", "")
								assets = append(assets, asset{Name: name, URL: "https://github.com/" + p.Repository + "/" + name})
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
				if strings.HasSuffix(r.URL.Path, "manifest.json") {
					sum := fmt.Sprintf("%x", sha256.Sum256([]byte("binary")))
					fmt.Fprintf(w, `{"platforms":{"linux-x64":{"checksum":"%s"},"linux-arm64":{"checksum":"%s"}}}`, sum, sum)
					return
				}
				if strings.HasSuffix(r.URL.Path, "chronology.html") {
					fmt.Fprint(w, `<tr><td>2026-10-09</td><td><a href="releaselog/3_54_0.html">3.54.0</a></td></tr>`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "download.html") {
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
