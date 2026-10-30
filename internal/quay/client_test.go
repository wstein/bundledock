package quay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tagServer(t *testing.T, pages [][]Tag) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("REST read sent credentials")
		}
		var page int
		fmt.Sscan(r.URL.Query().Get("page"), &page)
		if page < 1 || page > len(pages) {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"page": page, "tags": pages[page-1], "has_additional": page < len(pages)})
	}))
}

func TestTagsPaginates(t *testing.T) {
	srv := tagServer(t, [][]Tag{{{Name: "a"}}, {{Name: "b"}}})
	defer srv.Close()
	tags, err := New(srv.Client(), srv.URL).Tags(context.Background(), "toolbox", "")
	if err != nil || len(tags) != 2 || tags[1].Name != "b" {
		t.Fatalf("tags = %v, err = %v", tags, err)
	}
}

func TestTagsRejectsDuplicate(t *testing.T) {
	srv := tagServer(t, [][]Tag{{{Name: "a"}}, {{Name: "a"}}})
	defer srv.Close()
	if _, err := New(srv.Client(), srv.URL).Tags(context.Background(), "toolbox", ""); err == nil {
		t.Fatal("duplicate tag accepted")
	}
}

func TestTagsRejectsEndlessPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var page int
		fmt.Sscan(r.URL.Query().Get("page"), &page)
		json.NewEncoder(w).Encode(map[string]any{"page": page, "tags": []Tag{{Name: fmt.Sprint("t", page)}}, "has_additional": true})
	}))
	defer srv.Close()
	if _, err := New(srv.Client(), srv.URL).Tags(context.Background(), "toolbox", ""); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("err = %v", err)
	}
}

func TestResponseLimitAndStatus(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), maxBody+10))
	}))
	defer big.Close()
	if _, err := New(big.Client(), big.URL).Repository(context.Background(), "toolbox"); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v", err)
	}
	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	_, err := New(missing.Client(), missing.URL).Repository(context.Background(), "toolbox")
	var status *StatusError
	if !asStatus(err, &status) || status.Code != 404 {
		t.Fatalf("err = %v", err)
	}
}

func asStatus(err error, target **StatusError) bool {
	s, ok := err.(*StatusError)
	*target = s
	return ok
}

func TestNoRedirects(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := New(redirect.Client(), redirect.URL).Repository(context.Background(), "toolbox"); err == nil || hit {
		t.Fatalf("redirect followed (hit=%v, err=%v)", hit, err)
	}
}

func TestRepository(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository/bundledock/toolbox" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"description":"hello","is_public":true}`))
	}))
	defer srv.Close()
	repo, err := New(srv.Client(), srv.URL).Repository(context.Background(), "toolbox")
	if err != nil || repo.Description != "hello" || !repo.IsPublic {
		t.Fatalf("repo = %v, err = %v", repo, err)
	}
}

func TestLoadRecord(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	if r, err := LoadRecord(root, "nixbox"); r != nil || err != nil {
		t.Fatalf("absent record: %v %v", r, err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	os.WriteFile(filepath.Join(root, "docs", "toolbox-published.json"), []byte(`{"bundle":"toolbox","image":"quay.io/bundledock/toolbox","tag":"2026.10.30-b3","digest":"`+digest+`","extra":1}`), 0o644)
	r, err := LoadRecord(root, "toolbox")
	if err != nil || r.Tag != "2026.10.30-b3" || r.Digest != digest {
		t.Fatalf("record = %v, err = %v", r, err)
	}
	os.WriteFile(filepath.Join(root, "docs", "bun-published.json"), []byte(`{"bundle":"bun","tag":"x","digest":"bad"}`), 0o644)
	if _, err := LoadRecord(root, "bun"); err == nil {
		t.Fatal("invalid record accepted")
	}
}

func TestBundles(t *testing.T) {
	names := map[string]Bundle{}
	for _, b := range Bundles() {
		names[b.Name] = b
	}
	if len(names) != 10 || names["nixbox"].Repository != "nixbox" || names["node-current"].Repository != "node" || names["node-current"].Baseline != "current" || names["toolbox"].Baseline != "stable" {
		t.Fatalf("bundles = %v", names)
	}
	if _, err := FindBundle("nope"); err == nil {
		t.Fatal("unknown bundle accepted")
	}
}

func TestWriteJSON(t *testing.T) {
	var out bytes.Buffer
	if err := WriteJSON(&out, "bdock.quay.test/v1", struct {
		A int `json:"a"`
	}{1}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(out.Bytes(), &got)
	if got["schema"] != "bdock.quay.test/v1" || got["a"] != float64(1) {
		t.Fatalf("got %v", got)
	}
}
