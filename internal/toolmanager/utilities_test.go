package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestUtilitiesResolveOfficialStableBothArchitectures(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fzf", "bat", "lsd", "regctl", "mise", "trufflehog"} {
		t.Run(name, func(t *testing.T) {
			p := c.Catalog.Packages[name]
			payload := []byte("synthetic official artifact")
			sum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
			downloads := 0
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/repos/") {
					var releases []any
					for _, tag := range []string{c.Lock.Platforms["amd64"][name].Version, "v99.0.0-rc1"} {
						var assets []any
						for _, arch := range []string{"amd64", "arm64"} {
							asset := strings.NewReplacer("{version}", strings.TrimPrefix(tag, "v"), "{arch}", p.Architectures[arch]).Replace(p.Asset)
							assets = append(assets, map[string]string{"name": asset, "browser_download_url": "https://github.com/" + p.Repository + "/releases/download/" + tag + "/" + asset, "digest": sum})
						}
						releases = append(releases, map[string]any{"tag_name": tag, "assets": assets})
					}
					if err := json.NewEncoder(w).Encode(releases); err != nil {
						t.Fatal(err)
					}
					return
				}
				downloads++
				w.Write(payload)
			})
			pins, err := (resolver{client}).resolve(context.Background(), p, c.Desired[name])
			if err != nil {
				t.Fatal(err)
			}
			if downloads != 2 {
				t.Fatalf("downloads: %d", downloads)
			}
			for _, arch := range []string{"amd64", "arm64"} {
				if pins[arch].Version != c.Lock.Platforms[arch][name].Version || pins[arch].Verification != "publisher-sha256" {
					t.Fatalf("bad %s pin: %+v", arch, pins[arch])
				}
			}
		})
	}
}

func TestMiseResolvePublisherSidecarWithoutAPIDigest(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["mise"]
	payload := []byte("synthetic official mise")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))
	for _, digest := range []any{nil, "absent"} {
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/repos/") {
				var assets []any
				for _, arch := range []string{"amd64", "arm64"} {
					name := strings.NewReplacer("{version}", "2026.10.8", "{arch}", p.Architectures[arch]).Replace(p.Asset)
					a := map[string]any{"name": name, "browser_download_url": "https://github.com/jdx/mise/releases/download/v2026.10.8/" + name}
					if digest == nil {
						a["digest"] = nil
					}
					assets = append(assets, a)
				}
				assets = append(assets, map[string]string{"name": "SHASUMS256.txt", "browser_download_url": "https://github.com/jdx/mise/releases/download/v2026.10.8/SHASUMS256.txt"})
				if err := json.NewEncoder(w).Encode([]any{map[string]any{"tag_name": "v2026.10.8", "assets": assets}}); err != nil {
					t.Fatal(err)
				}
			} else if strings.HasSuffix(r.URL.Path, "/SHASUMS256.txt") {
				fmt.Fprintf(w, "%s  ./mise-v2026.10.8-linux-x64-musl\n%s  ./mise-v2026.10.8-linux-arm64-musl\n", sum, sum)
			} else {
				w.Write(payload)
			}
		})
		pins, err := (resolver{client}).resolve(context.Background(), p, c.Desired["mise"])
		if err != nil {
			t.Fatal(err)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if pins[arch].SHA256 != sum || pins[arch].Verification != "publisher-sha256" {
				t.Fatalf("unverified %s pin: %+v", arch, pins[arch])
			}
		}
	}
}

func TestSidecarRejectsForeignPaths(t *testing.T) {
	sum := strings.Repeat("a", 64)
	for _, path := range []string{"../mise", "/mise", "././mise", "dir/mise"} {
		if _, err := parseSidecar([]byte(sum+"  "+path), "mise"); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}
