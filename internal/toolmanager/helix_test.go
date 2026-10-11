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

func TestHelixUpdateRetainsCalendarAssetSpelling(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("synthetic upstream archive")
	sum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	var requested []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/") {
			var releases []any
			for _, tag := range []string{"25.07.1", "25.07.2", "25.08.1-rc1", "25.06.9"} {
				var assets []any
				for _, arch := range []string{"x86_64", "aarch64"} {
					name := "helix-" + tag + "-" + arch + "-linux.tar.xz"
					assets = append(assets, map[string]string{"name": name, "browser_download_url": "https://github.com/helix-editor/helix/releases/download/" + tag + "/" + name, "digest": sum})
				}
				releases = append(releases, map[string]any{"tag_name": tag, "assets": assets})
			}
			if err := json.NewEncoder(w).Encode(releases); err != nil {
				t.Fatal(err)
			}
			return
		}
		requested = append(requested, r.URL.Path)
		w.Write(payload)
	})
	desired, err := toolconfig.ParseDesired(strings.NewReader("helix>=25.7.0,<25.8.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	pins, err := (resolver{client}).resolve(context.Background(), c.Catalog.Packages["helix"], desired["helix"])
	if err != nil {
		t.Fatal(err)
	}
	if len(requested) != 2 {
		t.Fatalf("downloads: %v", requested)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		s := pins[arch]
		if s.Version != "25.07.2" || !strings.Contains(s.URL, "/25.07.2/helix-25.07.2-") || s.Verification != "publisher-sha256" {
			t.Fatalf("%s pin: %+v", arch, s)
		}
	}
}
