package toolpublish

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestNodeChannelPromotionIsolation(t *testing.T) {
	for _, name := range []string{"node", "node-current"} {
		t.Run(name, func(t *testing.T) {
			b, _ := SelectBundle(name)
			image, err := RepositoryImageForBundle("quay.io/example", name)
			if err != nil || image != "quay.io/example/node" {
				t.Fatalf("shared repository: %s %v", image, err)
			}
			lock := "../../" + b.Directory + "/tools.lock.json"
			data, _ := os.ReadFile(lock)
			version, _ := SingletonVersion(data, name)
			m, err := NewMetadataForBundle(name, lock, image, version+"-b1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-12T00:00:00Z", "123", "1")
			if err != nil {
				t.Fatal(err)
			}
			f := fake("amd64")
			for _, alias := range []string{"stable", "latest", "lts", "current"} {
				f.remote[image+":"+alias] = "untouched"
			}
			var receipts []Receipt
			for _, arch := range []string{"amd64", "arm64"} {
				native, r := buildFixture(t, m, lock, arch)
				pushed, err := native.adapter().Push(m, lock, r)
				if err != nil {
					t.Fatal(err)
				}
				for key, value := range native.remote {
					f.remote[key] = value
				}
				receipts = append(receipts, pushed)
			}
			if _, err = f.adapter().Promote(m, lock, receipts); err != nil {
				t.Fatal(err)
			}
			var aliases []string
			for _, call := range f.calls {
				if prefix(call, "docker", "buildx", "imagetools", "create") && len(call) == 7 {
					aliases = append(aliases, strings.TrimPrefix(call[5], image+":"))
				}
			}
			want := []string{"stable", "lts", "latest"}
			forbidden := []string{"current"}
			if name == "node-current" {
				want = []string{"current"}
				forbidden = []string{"stable", "lts", "latest"}
			}
			if !reflect.DeepEqual(aliases, want) {
				t.Fatalf("aliases %v want %v", aliases, want)
			}
			for _, alias := range forbidden {
				if f.remote[image+":"+alias] != "untouched" {
					t.Fatalf("changed other channel %s", alias)
				}
			}
			bad := m
			bad.Image = "quay.io/example/node-current"
			if validate(bad, lock) == nil {
				t.Fatal("accepted wrong output repository")
			}
			bad = m
			bad.Bundle = ""
			if validate(bad, lock) == nil {
				t.Fatal("accepted missing new Node source identity")
			}
			bad = m
			bad.Bundle = "codex"
			if validate(bad, lock) == nil {
				t.Fatal("accepted wrong source identity")
			}
		})
	}
}

func TestNodeSharedSourceReservations(t *testing.T) {
	b, _ := SelectBundle("node-current")
	refs := "aaaa refs/tags/node/26.11.1-b3\nbbbb refs/tags/node-current/26.11.1-b7\ncccc refs/tags/node-current/26.11.1-b7^{}\ndddd refs/tags/codex/26.11.1-b99\n"
	tags, err := SourceTagReservations(b, []byte(refs))
	if err != nil {
		t.Fatal(err)
	}
	next, err := NextSingletonTag("26.11.1", append(tags, "26.11.1-b5"))
	if err != nil || next != "26.11.1-b8" {
		t.Fatalf("same upstream later LTS collision: %s %v", next, err)
	}
	lts, _ := SelectBundle("node")
	if !reflect.DeepEqual(lts.SourceTagNamespaces(), b.SourceTagNamespaces()) {
		t.Fatal("Node channels have different revision namespaces")
	}
}

func TestNodeAllocationSharesAuthenticatedRepository(t *testing.T) {
	for _, name := range []string{"node-current", "node"} {
		t.Run(name, func(t *testing.T) {
			lock, err := os.ReadFile("../../bundles/node-current/tools.lock.json")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := SelectBundle(name)
			reserved, err := SourceTagReservations(b, []byte("a refs/tags/node/26.11.1-b3\nb refs/tags/node-current/26.11.1-b7\n"))
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := `{"token":"read-only"}`
				if r.URL.Path == "/v2/auth" {
					if r.URL.Query().Get("scope") != "repository:example/node:pull" {
						t.Fatal("wrong shared scope")
					}
				} else {
					if r.URL.Path != "/v2/example/node/tags/list" || r.Header.Get("Authorization") != "Bearer read-only" {
						t.Fatal("wrong shared authenticated listing")
					}
					body = `{"name":"example/node","tags":["26.11.1-b5","current","lts","stable","latest"]}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			got, err := AllocateSingletonTag(context.Background(), client, "quay.io/example/node", lock, "robot", "secret", reserved...)
			if err != nil || got != "26.11.1-b8" {
				t.Fatalf("Current then LTS must reserve b8: %s %v", got, err)
			}
		})
	}
}

func TestCurrentBaselineRequiresOwnAliasAndSource(t *testing.T) {
	for _, identity := range []string{"node-current", "node", ""} {
		t.Run("identity="+identity, func(t *testing.T) {
			objects := map[string]string{}
			add := func(v any) string {
				data, _ := json.Marshal(v)
				digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
				objects[digest] = string(data)
				return digest
			}
			var manifests []any
			for _, arch := range []string{"amd64", "arm64"} {
				labels := map[string]string{"io.bundledock.bundle": identity, "io.bundledock.github.run-id": "123", "io.bundledock.github.run-attempt": "1", "org.opencontainers.image.version": "26.11.1-b1", "org.opencontainers.image.revision": strings.Repeat("a", 40), "io.bundledock.tools.lock-sha256": strings.Repeat("b", 64), platformsLabel: strings.Repeat("c", 64)}
				config := add(map[string]any{"architecture": arch, "os": "linux", "config": map[string]any{"Labels": labels}})
				manifest := add(map[string]any{"config": map[string]string{"digest": config}})
				manifests = append(manifests, map[string]any{"digest": manifest, "platform": map[string]string{"os": "linux", "architecture": arch}})
			}
			index := add(map[string]any{"manifests": manifests})
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := `{"token":"read-only"}`
				if r.URL.Path != "/v2/auth" {
					if r.Header.Get("Authorization") != "Bearer read-only" {
						t.Fatal("unauthenticated baseline")
					}
					ref := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
					if ref == "stable" || ref == "lts" || ref == "latest" {
						t.Fatal("Current read LTS baseline")
					}
					if ref == "current" {
						ref = index
					}
					body = objects[ref]
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			got, err := PublishedToolDigestForBundle(context.Background(), client, "node-current", "quay.io/example/node", "robot", "secret", nil)
			if identity == "node-current" {
				if err != nil || got != strings.Repeat("c", 64) {
					t.Fatalf("valid Current baseline: %s %v", got, err)
				}
			} else if err == nil {
				t.Fatal("accepted wrong/missing Current source identity")
			}
		})
	}
}
