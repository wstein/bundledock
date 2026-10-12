package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestGenericMinimumSelectsFutureStableRelease(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["gh"]
	for _, tc := range []struct {
		name, releases, want string
		rejected             bool
	}{
		{"future-major", `[{"tag_name":"v2.102.0"},{"tag_name":"v3.0.0"},{"tag_name":"v4.0.0-rc.1","prerelease":true},{"tag_name":"v5.0.0","draft":true}]`, "v3.0.0", false},
		{"below-minimum", `[{"tag_name":"v2.101.99"}]`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.releases) })
			rel, err := (resolver{client: client}).github(context.Background(), p, c.Desired["gh"])
			if tc.rejected {
				if err == nil {
					t.Fatal("selected below minimum")
				}
				return
			}
			if err != nil || rel.Tag != tc.want {
				t.Fatalf("selected %q, want %q: %v", rel.Tag, tc.want, err)
			}
		})
	}
}
