package toolpublish

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func singletonVersionFixture(name, upstream string) []byte {
	// Freeze parser fixtures independently of the live updater-owned locks.
	tool, prefix := name, "v"
	switch name {
	case "node-current":
		tool = "node"
	case "codex":
		prefix = "rust-v"
	case "bun":
		prefix = "bun-v"
	case "agy", "claude":
		prefix = ""
	}
	pin := fmt.Sprintf(`{"%s":{"version": "%s%s"}}`, tool, prefix, upstream)
	return []byte(fmt.Sprintf(`{"schema":2,"platforms":{"amd64":%s,"arm64":%s}}`, pin, pin))
}

func TestSingletonVersionAllocation(t *testing.T) {
	for _, test := range []struct{ name, upstream string }{
		{"opentofu", "1.13.1"}, {"codex", "0.162.1"}, {"agy", "1.3.3"}, {"claude", "2.1.296"},
		{"node-current", "26.11.1"}, {"node", "24.21.0"}, {"bun", "1.4.3"}, {"code-server", "4.141.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			lock := singletonVersionFixture(test.name, test.upstream)
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
	lock := filepath.Join(t.TempDir(), "codex.lock.json")
	if err := os.WriteFile(lock, singletonVersionFixture("codex", "0.162.1"), 0600); err != nil {
		t.Fatal(err)
	}
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
	lock := singletonVersionFixture("codex", "0.162.1")
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

func TestToolboxDateRevisionAllocation(t *testing.T) {
	for _, test := range []struct {
		date string
		tags []string
		want string
	}{
		{"2026.10.15", nil, "2026.10.15-b1"},
		{"2026.10.16", []string{"2026.10.15-b99"}, "2026.10.16-b1"},
		{"2026.10.15", []string{"2026.10.15-b98.1", "2026.10.15-123-1", "2026.10.14-b99"}, "2026.10.15-b1"},
		{"2026.10.15", []string{"2026.10.15-b2", "2026.10.15-b9", "staging-2026.10.15-b12-arm64"}, "2026.10.15-b13"},
	} {
		got, err := NextToolboxTag(test.date, test.tags)
		if err != nil || got != test.want {
			t.Fatalf("%s: %s, %v; want %s", test.date, got, err, test.want)
		}
	}
	for _, date := range []string{"2026.02.30", "2026.1.1", "", "2026.10.15/evil"} {
		if _, err := NextToolboxTag(date, nil); err == nil {
			t.Fatalf("accepted date %s", date)
		}
	}
	if _, err := NextToolboxTag("2026.10.15", []string{"2026.10.15-b18446744073709551615"}); err == nil {
		t.Fatal("revision overflow accepted")
	}
}

func TestToolboxDailyMetadataRequiresExplicitAttempt(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "lock.json")
	if err := os.WriteFile(lock, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"quay.io/example/toolbox", "2026.10.15-b1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-15T00:00:00Z", "123"}
	if _, err := NewMetadata(lock, args[0], args[1], args[2], args[3], args[4], args[5]); err == nil {
		t.Fatal("missing attempt accepted")
	}
	m, err := NewMetadata(lock, args[0], args[1], args[2], args[3], args[4], args[5], "2")
	if err != nil || m.RunAttempt != "2" {
		t.Fatalf("daily metadata: %+v %v", m, err)
	}
	if err := validate(m, lock); err != nil {
		t.Fatal(err)
	}
}

func TestToolboxRegistryAndSourceReservations(t *testing.T) {
	for _, mode := range []string{"valid", "unauthorized", "wrong-repository", "invalid-json", "missing-tags", "stalled"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				status, body := 200, `{"token":"token"}`
				headers := http.Header{}
				if calls > 1 {
					if r.URL.Path != "/v2/example/toolbox/tags/list" || r.Header.Get("Authorization") != "Bearer token" {
						t.Fatal("wrong registry scope")
					}
					body = `{"name":"example/toolbox","tags":["2026.10.15-b2","2026.10.15-b98.1"]}`
					if calls == 2 {
						headers.Set("Link", "next")
					} else {
						if r.URL.Query().Get("last") != "2026.10.15-b98.1" {
							t.Fatal("pagination skipped")
						}
						body = `{"name":"example/toolbox","tags":["staging-2026.10.15-b8-amd64"]}`
					}
					switch mode {
					case "unauthorized":
						status = 401
					case "wrong-repository":
						body = `{"name":"other/toolbox","tags":[]}`
					case "invalid-json":
						body = `bad`
					case "missing-tags":
						body = `{"name":"example/toolbox"}`
					case "stalled":
						body = `{"name":"example/toolbox","tags":[]}`
						headers.Set("Link", "next")
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers}, nil
			})}
			got, err := AllocateToolboxTag(context.Background(), client, "quay.io/example/toolbox", "2026.10.15", "robot", "secret", "2026.10.15-b12")
			if mode == "valid" {
				if err != nil || got != "2026.10.15-b13" {
					t.Fatalf("%s %v", got, err)
				}
			} else if err == nil {
				t.Fatal("incomplete registry proof accepted")
			}
		})
	}
}

func TestDailyTagCollision(t *testing.T) {
	for _, status := range []int{200, 404, 403} {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			code, body := 200, `{"token":"token"}`
			if r.Method == http.MethodHead {
				code = status
				body = ""
				if !strings.HasSuffix(r.URL.Path, "/2026.03.01-b1") {
					t.Fatal("wrong version checked")
				}
			}
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})}
		err := CheckTagAbsent(context.Background(), client, "quay.io/example/toolbox", "2026.03.01-b1", "robot", "secret")
		if (err == nil) != (status == 404) {
			t.Fatalf("HTTP%d: %v", status, err)
		}
	}
}
