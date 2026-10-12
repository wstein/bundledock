package toolpublish

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSingletonVersionAllocation(t *testing.T) {
	for _, test := range []struct{ name, upstream string }{
		{"codex", "0.162.1"}, {"agy", "1.3.3"}, {"claude", "2.1.296"},
		{"node-current", "26.11.1"}, {"node", "24.21.0"}, {"bun", "1.4.3"}, {"code-server", "4.141.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			lock, err := os.ReadFile("../../bundles/" + test.name + "/packages.lock.json")
			if err != nil {
				t.Fatal(err)
			}
			version, err := SingletonVersion(lock, test.name)
			if err != nil || version != test.upstream {
				t.Fatalf("version %q: %v", version, err)
			}
			for _, test := range []struct {
				tags []string
				want string
			}{
				{nil, version + "-b1"},
				{[]string{"2026.10.12-b999.1", version + "-b1", version + "-b9", "999.0.0-b44", "stage-123"}, version + "-b10"},
				{[]string{"0.0.1-b99"}, version + "-b1"},
			} {
				got, err := NextSingletonTag(version, test.tags)
				if err != nil || got != test.want {
					t.Fatalf("allocation %q, want %q: %v", got, test.want, err)
				}
			}
			// Both native architectures must identify the same official release.
			changed := strings.Replace(string(lock), `"version": "`, `"version": "9`, 1)
			if _, err := SingletonVersion([]byte(changed), test.name); err == nil {
				t.Fatal("accepted differing architecture versions")
			}
		})
	}
}

func TestSemanticMetadataBindsAttemptAndLock(t *testing.T) {
	lock := "../../bundles/codex/packages.lock.json"
	m, err := NewMetadata(lock, "quay.io/example/codex", "0.162.1-b1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-12T00:00:00Z", "123", "2")
	if err != nil {
		t.Fatal(err)
	}
	if m.RunAttempt != "2" {
		t.Fatalf("attempt missing: %+v", m)
	}
	if err := validate(m, lock); err != nil {
		t.Fatal(err)
	}
	m.Tag = "0.162.2-b1"
	if err := validate(m, lock); err == nil {
		t.Fatal("accepted tag differing from frozen lock")
	}
	if _, err := NewMetadata(lock, "quay.io/example/toolbox", "0.162.1-b1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-12T00:00:00Z", "123", "2"); err == nil {
		t.Fatal("toolbox accepted semantic version")
	}
}

func TestRegistryAllocatesAcrossPagesAndSourceReservations(t *testing.T) {
	lock, err := os.ReadFile("../../bundles/codex/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "unauthorized", "wrong-repository", "invalid-json", "stalled"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				status := 200
				body := `{"token":"token"}`
				headers := http.Header{}
				if calls > 1 {
					if r.URL.Path != "/v2/example/codex/tags/list" || r.Header.Get("Authorization") != "Bearer token" {
						t.Fatal("wrong scoped listing")
					}
					body = `{"name":"example/codex","tags":["0.162.1-b1","0.162.1-b2"]}`
					if calls == 2 {
						headers.Set("Link", `</v2/example/codex/tags/list?last=0.162.1-b2>; rel="next"`)
					} else {
						if r.URL.Query().Get("last") != "0.162.1-b2" {
							t.Fatal("pagination skipped versions")
						}
						body = `{"name":"example/codex","tags":["0.162.1-b5","2026.10.12-b999.7"]}`
					}
					switch mode {
					case "unauthorized":
						status = 401
					case "wrong-repository":
						body = `{"name":"example/evil","tags":[]}`
					case "invalid-json":
						body = `bad`
					case "stalled":
						body = `{"name":"example/codex","tags":[]}`
						headers.Set("Link", "next")
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers}, nil
			})}
			got, err := AllocateSingletonTag(context.Background(), client, "quay.io/example/codex", lock, "robot", "secret", "0.162.1-b7")
			if mode == "valid" {
				if err != nil || got != "0.162.1-b8" || calls != 3 {
					t.Fatalf("allocation %q calls%d: %v", got, calls, err)
				}
			} else if err == nil {
				t.Fatal("failed-open registry listing")
			}
		})
	}
}

func TestSemanticVersionCollisionStopsPublication(t *testing.T) {
	for _, status := range []int{200, 404, 403} {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			code := 200
			body := `{"token":"token"}`
			if calls == 2 {
				code = status
				body = ""
				if r.Method != "HEAD" || !strings.HasSuffix(r.URL.Path, "/0.162.1-b2") {
					t.Fatal("wrong version checked")
				}
			}
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})}
		err := CheckTagAbsent(context.Background(), client, "quay.io/example/codex", "0.162.1-b2", "robot", "secret")
		if (err == nil) != (status == 404) {
			t.Fatalf("status%d accepted: %v", status, err)
		}
	}
}
