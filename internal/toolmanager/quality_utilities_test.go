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

func TestQualityUtilitiesResolveOfficialStableBothArchitectures(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"shellcheck", "shfmt", "just", "ruff", "delta", "watchexec", "hyperfine", "age", "jj", "lazygit"} {
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
