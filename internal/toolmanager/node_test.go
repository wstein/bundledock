package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"

	"testing"
)

func TestNodeResolverRequiresLTSBothPrebuiltsAndPublisherSums(t *testing.T) {
	for _, kind := range []string{"ok", "missing arm64", "bad checksum", "ambiguous checksum", "no LTS"} {
		t.Run(kind, func(t *testing.T) {
			c, err := toolconfig.Load("../../bundles/node")
			if err != nil {
				t.Fatal(err)
			}
			client := fixtureClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/dist/index.json":
					files := `["linux-x64","linux-arm64"]`
					if kind == "missing arm64" {
						files = `["linux-x64"]`
					}
					lts := `"Krypton"`
					if kind == "no LTS" {
						lts = `false`
					}
					fmt.Fprintf(w, `[{"version":"v26.10.0","lts":false},{"version":"v24.21.0","lts":%s,"files":%s}]`, lts, files)
				case "/dist/v24.21.0/SHASUMS256.txt":
					sum := fmt.Sprintf("%x", sha256.Sum256([]byte("fixture")))
					if kind == "bad checksum" {
						sum = fmt.Sprintf("%064d", 0)
					}
					for _, a := range []string{"x64", "arm64"} {
						fmt.Fprintf(w, "%s  node-v24.21.0-linux-%s.tar.gz\n", sum, a)
					}
					if kind == "ambiguous checksum" {
						fmt.Fprintf(w, "%s  node-v24.21.0-linux-x64.tar.gz\n", sum)
					}
				case "/dist/v24.21.0/node-v24.21.0-linux-x64.tar.gz", "/dist/v24.21.0/node-v24.21.0-linux-arm64.tar.gz":
					fmt.Fprint(w, "fixture")
				default:
					t.Errorf("unexpected upstream path %s", r.URL.Path)
					http.Error(w, "unexpected", 404)
				}
			}))
			pins, err := (resolver{client}).resolve(context.Background(), c.Catalog.Packages["node"], c.Desired["node"])
			if kind != "ok" {
				if err == nil {
					t.Fatal("unsafe resolution accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(pins) != 2 || pins["amd64"].Version != "v24.21.0" || pins["arm64"].Verification != "publisher-sha256" {
				t.Fatalf("bad pins %+v", pins)
			}
		})
	}
}
