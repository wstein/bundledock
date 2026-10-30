package quay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDescription(t *testing.T, root, bundle, text string) {
	t.Helper()
	dir := filepath.Join(root, "bundles", bundle)
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDescription(t *testing.T) {
	root := t.TempDir()
	if _, ok, err := LoadDescription(root, "toolbox"); ok || err != nil {
		t.Fatalf("absent: %v %v", ok, err)
	}
	writeDescription(t, root, "toolbox", "hello\n")
	if text, ok, err := LoadDescription(root, "toolbox"); text != "hello" || !ok || err != nil {
		t.Fatalf("got %q %v %v", text, ok, err)
	}
	writeDescription(t, root, "toolbox", strings.Repeat("x", MaxDescription+1))
	if _, _, err := LoadDescription(root, "toolbox"); err == nil {
		t.Fatal("too long accepted")
	}
}

func TestDescribeCheckAndApply(t *testing.T) {
	live := map[string]string{"toolbox": "old", "nixbox": "same"}
	var puts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo := strings.TrimPrefix(r.URL.Path, "/api/v1/repository/bundledock/")
		switch r.Method {
		case http.MethodGet:
			if r.Header.Get("Authorization") != "" {
				t.Error("GET sent credentials")
			}
			json.NewEncoder(w).Encode(map[string]any{"description": live[repo], "is_public": true})
		case http.MethodPut:
			if r.Header.Get("Authorization") != "Bearer secret-token" {
				w.WriteHeader(401)
				return
			}
			var body map[string]string
			raw, _ := io.ReadAll(r.Body)
			json.Unmarshal(raw, &body)
			live[repo] = body["description"]
			puts = append(puts, repo)
			w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()
	root := t.TempDir()
	writeDescription(t, root, "toolbox", "new")
	writeDescription(t, root, "nixbox", "same")
	c := New(srv.Client(), srv.URL)
	rows, err := Describe(context.Background(), c, root, Bundles(), false, "")
	if err != nil || len(puts) != 0 {
		t.Fatalf("check wrote or failed: %v %v", err, puts)
	}
	states := map[string]string{}
	for _, r := range rows {
		states[r.Bundle] = r.State
	}
	if states["toolbox"] != "differs" || states["nixbox"] != "same" || states["bun"] != "skipped" {
		t.Fatalf("states = %v", states)
	}
	if _, err := Describe(context.Background(), c, root, Bundles(), true, ""); err == nil {
		t.Fatal("apply without a token accepted")
	}
	rows, err = Describe(context.Background(), c, root, Bundles(), true, "secret-token")
	if err != nil || len(puts) != 1 || puts[0] != "toolbox" || live["toolbox"] != "new" {
		t.Fatalf("apply: %v puts=%v live=%v", err, puts, live)
	}
	for _, r := range rows {
		if r.Bundle == "toolbox" && r.State != "applied" {
			t.Fatalf("row = %+v", r)
		}
	}
}

func TestSetDescriptionErrorHidesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte("bad token secret-token"))
	}))
	defer srv.Close()
	err := New(srv.Client(), srv.URL).SetDescription(context.Background(), "toolbox", "secret-token", "x")
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("err = %v", err)
	}
}

func TestDescribeNonPublicDiffers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"description":"same","is_public":false}`))
	}))
	defer srv.Close()
	root := t.TempDir()
	writeDescription(t, root, "toolbox", "same")
	b, _ := FindBundle("toolbox")
	rows, err := Describe(context.Background(), New(srv.Client(), srv.URL), root, []Bundle{b}, false, "")
	if err != nil || rows[0].State != "differs" {
		t.Fatalf("rows = %v err = %v", rows, err)
	}
}
