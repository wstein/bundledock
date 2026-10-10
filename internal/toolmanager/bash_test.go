package toolmanager

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestBashUpgradeDiscoversOrderedPatchesAndKeepsAtomicLock(t *testing.T) {
	for _, failure := range []string{"", "patch-gap", "source-http", "patch-http", "output"} {
		t.Run(failure, func(t *testing.T) {
			dir := checkout(t)
			before := readLock(t, dir)
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/gnu/bash/":
					fmt.Fprint(w, `<a href="bash-5.3.tar.gz">base</a><a href="bash-5.3-patches/">patches</a><a href="bash-5.4.tar.gz">outside range</a>`)
				case "/gnu/bash/bash-5.3-patches/":
					for i := 21; i >= 1; i-- {
						if failure == "patch-gap" && i == 5 {
							continue
						}
						fmt.Fprintf(w, `<a href="bash53-%03d">patch</a><a href="bash53-%03d.sig">signature</a>`, i, i)
					}
				default:
					if failure == "source-http" && strings.HasSuffix(r.URL.Path, ".tar.gz") || failure == "patch-http" && strings.HasSuffix(r.URL.Path, "-021") {
						http.Error(w, "unavailable", 503)
						return
					}
					fmt.Fprint(w, "GNU fixture")
				}
			})
			var out bytes.Buffer
			var writer interface{ Write([]byte) (int, error) } = &out
			if failure == "output" {
				writer = failedWriter{}
			}
			err := Upgrade(context.Background(), client, dir, "bash", writer)
			if failure != "" {
				if err == nil {
					t.Fatal("expected failure")
				}
				if !bytes.Equal(before, readLock(t, dir)) {
					t.Fatal("failure rewrote lock")
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
				s := c.Lock.Platforms[arch]["bash"]
				if s.Version != "5.3.21" || len(s.Patches) != 21 {
					t.Fatalf("patch update not resolved: %+v", s)
				}
				for i, p := range s.Patches {
					if !strings.HasSuffix(p.URL, fmt.Sprintf("-%03d", i+1)) || p.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte("GNU fixture"))) {
						t.Fatal("patch ordering/checksum")
					}
				}
			}
		})
	}
}
