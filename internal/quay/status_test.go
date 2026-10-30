package quay

import (
	"context"
	"testing"
)

func TestCompare(t *testing.T) {
	b, _ := FindBundle("toolbox")
	rec := &Record{Bundle: "toolbox", Tag: "2026.10.30-b3", Digest: "sha256:aa"}
	good := []Tag{{Name: "2026.10.30-b3", ManifestDigest: "sha256:aa"}, {Name: "latest", ManifestDigest: "sha256:aa"}, {Name: "stable", ManifestDigest: "sha256:aa"}}
	if row := Compare(b, rec, good); row.State != "ok" || len(row.Checks) != 3 {
		t.Fatalf("good = %+v", row)
	}
	if row := Compare(b, nil, nil); row.State != "skipped" {
		t.Fatalf("skipped = %+v", row)
	}
	stale := []Tag{{Name: "2026.10.30-b3", ManifestDigest: "sha256:aa"}, {Name: "latest", ManifestDigest: "sha256:aa"}, {Name: "stable", ManifestDigest: "sha256:bb"}}
	if row := Compare(b, rec, stale); row.State != "differs" {
		t.Fatalf("stale alias = %+v", row)
	}
	missing := good[1:]
	if row := Compare(b, rec, missing); row.State != "differs" {
		t.Fatalf("missing version = %+v", row)
	}
	residue := append([]Tag{{Name: "staging-2026.10.30-b3-amd64", ManifestDigest: "sha256:cc"}}, good...)
	row := Compare(b, rec, residue)
	if row.State != "differs" || row.Checks[len(row.Checks)-1].Kind != "residue" {
		t.Fatalf("residue = %+v", row)
	}
	other := append([]Tag{{Name: "staging-2026.10.30-b4-amd64", ManifestDigest: "sha256:cc"}}, good...)
	if row := Compare(b, rec, other); row.State != "ok" {
		t.Fatalf("staging of an unpublished version is not residue: %+v", row)
	}
}

func TestStatusSkipsUnpublished(t *testing.T) {
	srv := fakeQuay(t, nil)
	defer srv.Close()
	n, _ := FindBundle("nixbox")
	rows, err := Status(context.Background(), &Inventory{Client: New(srv.Client(), srv.URL)}, t.TempDir(), []Bundle{n})
	if err != nil || rows[0].State != "skipped" {
		t.Fatalf("rows = %v, err = %v", rows, err)
	}
}
