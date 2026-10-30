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

func TestHelixMinimumUpdateRetainsCalendarAssetSpelling(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("synthetic upstream archive")
	sum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	var requested []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/") {
			var releases []any
			for _, tag := range []string{"25.07.1", "26.01.1", "26.02.1-rc1", "25.06.9"} {
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
	pins, err := (resolver{client}).resolve(context.Background(), c.Catalog.Packages["helix"], c.Desired["helix"])
	if err != nil {
		t.Fatal(err)
	}
	if len(requested) != 2 {
		t.Fatalf("downloads: %v", requested)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		s := pins[arch]
		if s.Version != "26.01.1" || !strings.Contains(s.URL, "/26.01.1/helix-26.01.1-") || s.Verification != "publisher-sha256" {
			t.Fatalf("%s pin: %+v", arch, s)
		}
	}
}
