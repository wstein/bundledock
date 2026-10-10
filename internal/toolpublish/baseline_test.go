package toolpublish

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func testLock(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../bundles/toolbox/tools.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestToolDigestTracksAssetsNotInputRanges(t *testing.T) {
	initial := testLock(t)
	want, err := ToolDigest(initial)
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	if err := json.Unmarshal(initial, &lock); err != nil {
		t.Fatal(err)
	}
	lock["desired_digest"], lock["catalog_digest"] = "changed range", "changed recipe metadata"
	changed, _ := json.Marshal(lock)
	if got, err := ToolDigest(changed); err != nil || got != want {
		t.Fatalf("input-only change rebuilt: %s %v", got, err)
	}
	platforms := lock["platforms"].(map[string]any)
	tool := platforms["arm64"].(map[string]any)["gh"].(map[string]any)
	tool["sha256"] = strings.Repeat("a", 64)
	changed, _ = json.Marshal(lock)
	if got, err := ToolDigest(changed); err != nil || got == want {
		t.Fatalf("asset change omitted: %s %v", got, err)
	}
	for _, invalid := range []string{`{}`, `{"schema":2,"platforms":{"amd64":{},"arm64":{}}}`, `{"schema":99}`} {
		if _, err := ToolDigest([]byte(invalid)); err == nil {
			t.Fatalf("accepted incomplete baseline %s", invalid)
		}
	}
}

func registryBaseline(t *testing.T, semantic string, legacy bool, corrupt bool) (*http.Client, Metadata) {
	t.Helper()
	lock := testLock(t)
	m := Metadata{Image: "quay.io/example/toolbox", RunID: "123", Tag: "2026.10.10-b12.4", Commit: strings.Repeat("a", 40), Source: "https://github.com/example/repo", Created: "2026-10-10T03:17:00Z", LockSHA256: fmt.Sprintf("%x", sha256.Sum256(lock))}
	if legacy {
		m.Tag = "2026.10.10-123-4"
	}
	objects := map[string]string{}
	object := func(v any) string {
		b, _ := json.Marshal(v)
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(b))
		objects[digest] = string(b)
		return digest
	}
	var manifests []any
	for _, arch := range []string{"amd64", "arm64"} {
		labels := map[string]string{"io.bundledock.tools.lock-sha256": m.LockSHA256, "io.bundledock.github.run-id": m.RunID, "org.opencontainers.image.version": m.Tag, "org.opencontainers.image.revision": m.Commit, "org.opencontainers.image.source": m.Source, "org.opencontainers.image.created": m.Created}
		if !legacy {
			labels["io.bundledock.tools.platforms-sha256"] = semantic
		} else {
			// The actual first published image predates the run-id label.
			delete(labels, "io.bundledock.github.run-id")
		}
		if corrupt && arch == "arm64" {
			labels["io.bundledock.tools.lock-sha256"] = strings.Repeat("b", 64)
		}
		config := object(map[string]any{"architecture": arch, "os": "linux", "config": map[string]any{"Labels": labels}})
		manifest := object(map[string]any{"config": map[string]string{"digest": config}})
		manifests = append(manifests, map[string]any{"digest": manifest, "platform": map[string]string{"os": "linux", "architecture": arch}})
	}
	index := object(map[string]any{"manifests": manifests})
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "quay.io" || r.Method != "GET" {
			t.Fatalf("unexpected registry operation %s %s", r.Method, r.URL)
		}
		body, status := "", 200
		if r.URL.Path == "/v2/auth" {
			if r.URL.Query().Get("scope") != "repository:example/toolbox:pull" {
				t.Fatal("non-pull registry scope")
			}
			body = `{"token":"read-token"}`
		} else {
			if r.Header.Get("Authorization") != "Bearer read-token" {
				t.Fatal("missing read authorization")
			}
			key := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if key == "stable" {
				key = index
			}
			var ok bool
			body, ok = objects[key]
			if !ok {
				status = 404
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}, m
}

func TestPublishedBaselineDurableAndLegacy(t *testing.T) {
	lock := testLock(t)
	want, _ := ToolDigest(lock)
	for _, legacy := range []bool{false, true} {
		client, meta := registryBaseline(t, want, legacy, false)
		calls := 0
		got, err := PublishedToolDigest(context.Background(), client, meta.Image, "robot", "secret", func(m Metadata) ([]byte, error) {
			calls++
			if m != meta {
				t.Fatalf("unbound legacy provenance: %+v", m)
			}
			return lock, nil
		})
		if err != nil || got != want || (legacy && calls != 1) || (!legacy && calls != 0) {
			t.Fatalf("legacy=%t: %s %v calls=%d", legacy, got, err, calls)
		}
	}
}

func TestPublishedBaselineFailsClosed(t *testing.T) {
	lock := testLock(t)
	want, _ := ToolDigest(lock)
	for _, scenario := range []string{"mismatched platforms", "expired artifact", "wrong artifact", "missing stable", "unauthorized", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			client, meta := registryBaseline(t, want, true, scenario == "mismatched platforms")
			if scenario == "missing stable" || scenario == "unauthorized" || scenario == "redirect" {
				status := 404
				if scenario == "unauthorized" {
					status = 401
				}
				if scenario == "redirect" {
					status = 302
				}
				client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{"Location": []string{"https://evil.invalid"}}}, nil
				})}
			}
			_, err := PublishedToolDigest(context.Background(), client, meta.Image, "robot", "secret", func(Metadata) ([]byte, error) {
				if scenario == "expired artifact" {
					return nil, fmt.Errorf("expired")
				}
				return []byte(`{}`), nil
			})
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe baseline result %v", err)
			}
		})
	}
}
