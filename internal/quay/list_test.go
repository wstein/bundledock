package quay

import (
	"context"
	"testing"
)

func TestPatterns(t *testing.T) {
	for name, want := range map[string]bool{"2026.10.30-b3": true, "2026.10.30-b3.2": true, "1.4.10-b1": true, "latest": false, "staging-1.4.10-b1-amd64": false} {
		if IsVersion(name) != want {
			t.Errorf("IsVersion(%s) != %v", name, want)
		}
	}
	if !IsStaging("staging-1.4.10-b1-arm64") || IsStaging("staging-x") || IsStaging("stable") {
		t.Error("IsStaging wrong")
	}
}

func TestNewest(t *testing.T) {
	tags := []Tag{
		{Name: "26.12.0-b1", ManifestDigest: "d3", LastModified: "Fri, 30 Oct 2026 10:00:00 -0000"},
		{Name: "24.22.2-b1", ManifestDigest: "d1", LastModified: "Thu, 29 Oct 2026 10:00:00 -0000"},
		{Name: "24.22.1-b1", ManifestDigest: "d0", LastModified: "Wed, 28 Oct 2026 10:00:00 -0000"},
		{Name: "stable", ManifestDigest: "d1"},
		{Name: "current", ManifestDigest: "d3"},
	}
	node, _ := FindBundle("node")
	if got, ok := Newest(tags, node); !ok || got.Name != "24.22.2-b1" {
		t.Fatalf("node newest = %v", got)
	}
	cur, _ := FindBundle("node-current")
	if got, ok := Newest(tags, cur); !ok || got.Name != "26.12.0-b1" {
		t.Fatalf("current newest = %v", got)
	}
	if _, ok := Newest(nil, node); ok {
		t.Fatal("empty has newest")
	}
}

func TestList(t *testing.T) {
	empty := []Tag{}
	srv := fakeQuay(t, map[string][]Tag{"code-server": empty, "node": empty, "bun": empty, "codex": empty, "agy": empty, "claude": empty, "opentofu": empty, "nixbox": empty, "toolbox": {{Name: "2026.10.30-b3", ManifestDigest: "d", LastModified: "Fri, 30 Oct 2026 10:00:00 -0000"}, {Name: "staging-2026.10.30-b3-amd64"}}})
	defer srv.Close()
	rows, err := List(context.Background(), &Inventory{Client: New(srv.Client(), srv.URL)}, repoRoot(t))
	if err != nil || len(rows) != 10 {
		t.Fatalf("rows = %v, err = %v", rows, err)
	}
	for _, r := range rows {
		if r.Bundle == "nixbox" && (r.Version != "" || r.Staging != 0) {
			t.Fatalf("nixbox row = %v", r)
		}
		if r.Bundle == "toolbox" && (r.Version != "2026.10.30-b3" || r.Staging != 1 || !r.Published) {
			t.Fatalf("toolbox row = %v", r)
		}
	}
}

func TestListUnpublishedMissingRepository(t *testing.T) {
	srv := fakeQuay(t, nil)
	defer srv.Close()
	rows, err := List(context.Background(), &Inventory{Client: New(srv.Client(), srv.URL)}, t.TempDir())
	if err != nil || len(rows) != 10 || rows[0].Published || rows[0].Version != "" {
		t.Fatalf("rows = %v, err = %v", rows, err)
	}
}

func TestListPublishedMissingRepositoryFails(t *testing.T) {
	srv := fakeQuay(t, nil)
	defer srv.Close()
	if _, err := List(context.Background(), &Inventory{Client: New(srv.Client(), srv.URL)}, repoRoot(t)); err == nil {
		t.Fatal("published bundle with a missing repository accepted")
	}
}
