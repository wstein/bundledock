package main

import (
	"bundledock/internal/quay"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type quayTag struct {
	Name           string `json:"name"`
	ManifestDigest string `json:"manifest_digest"`
	LastModified   string `json:"last_modified"`
}

// fakeQuayAPI serves the tag listing of the given repositories (empty for the
// other bundle repositories) so tests never reach the network.
func fakeQuayAPI(t *testing.T, repos map[string][]quayTag) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, _ := strings.CutPrefix(r.URL.Path, "/api/v1/repository/bundledock/")
		repo, suffix, _ := strings.Cut(rest, "/")
		if suffix != "tag/" {
			http.NotFound(w, r)
			return
		}
		tags := repos[repo]
		if tags == nil {
			tags = []quayTag{}
		}
		json.NewEncoder(w).Encode(map[string]any{"page": 1, "tags": tags, "has_additional": false})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runQuay(t *testing.T, srv *httptest.Server, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runQuayAt(t, "../..", srv, args...)
}

func runQuayAt(t *testing.T, root string, srv *httptest.Server, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	target, _ := url.Parse(srv.URL)
	// The command talks to quay.io only; the test transport sends it to the fake.
	cmd := newRootCommandWithClient(&http.Client{Transport: rewriteTransport{target}})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"quay"}, append(args, "--root", root)...))
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestQuayList(t *testing.T) {
	srv := fakeQuayAPI(t, map[string][]quayTag{"toolbox": {
		{Name: "2026.10.30-b3", ManifestDigest: "sha256:aa", LastModified: "Fri, 30 Oct 2026 10:00:00 -0000"},
		{Name: "staging-2026.10.30-b4-amd64", ManifestDigest: "sha256:bb"},
		{Name: "staging-2026.10.30-b4-arm64", ManifestDigest: "sha256:cc"},
	}})
	out, _, err := runQuay(t, srv, "list")
	if err != nil || !strings.Contains(out, "toolbox") || !strings.Contains(out, "2026.10.30-b3") || !strings.Contains(out, "sha256:aa") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	out, _, err = runQuay(t, srv, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Schema  string `json:"schema"`
		Bundles []struct {
			Bundle  string `json:"bundle"`
			Version string `json:"version"`
			Digest  string `json:"digest"`
			Staging int    `json:"staging_tags"`
		} `json:"bundles"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Schema != "bdock.quay.list/v1" || len(doc.Bundles) != 10 {
		t.Fatalf("doc = %+v, err = %v", doc, err)
	}
	for _, b := range doc.Bundles {
		if b.Bundle == "toolbox" && (b.Version != "2026.10.30-b3" || b.Staging != 2) {
			t.Fatalf("toolbox = %+v", b)
		}
	}
}

// writeRecord writes a publication record below a temporary root.
func writeRecord(t *testing.T, root, bundle, tag, digest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	record := `{"bundle":"` + bundle + `","image":"quay.io/bundledock/` + bundle + `","tag":"` + tag + `","digest":"` + digest + `"}`
	if err := os.WriteFile(filepath.Join(root, "docs", bundle+"-published.json"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQuayStatus(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	root := t.TempDir()
	writeRecord(t, root, "toolbox", "2026.10.30-b3", digest)
	good := []quayTag{{Name: "2026.10.30-b3", ManifestDigest: digest}, {Name: "latest", ManifestDigest: digest}, {Name: "stable", ManifestDigest: digest}}
	srv := fakeQuayAPI(t, map[string][]quayTag{"toolbox": good})
	out, _, err := runQuayAt(t, root, srv, "status")
	if err != nil || !strings.Contains(out, "toolbox") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	// A moved alias is a difference, with and without --json.
	bad := append([]quayTag{}, good[:2]...)
	bad = append(bad, quayTag{Name: "stable", ManifestDigest: "sha256:" + strings.Repeat("b", 64)})
	srv = fakeQuayAPI(t, map[string][]quayTag{"toolbox": bad})
	for _, args := range [][]string{{"status"}, {"status", "--json"}, {"status", "toolbox"}} {
		out, _, err = runQuayAt(t, root, srv, args...)
		if err == nil {
			t.Fatalf("%v: expected exit 1, out = %q", args, out)
		}
	}
	out, _, _ = runQuayAt(t, root, srv, "status", "--json")
	var doc struct {
		Schema  string `json:"schema"`
		Bundles []struct {
			Bundle, State string
		} `json:"bundles"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Schema != "bdock.quay.status/v1" || len(doc.Bundles) != 10 || doc.Bundles[0].State != "differs" || doc.Bundles[1].State != "skipped" {
		t.Fatalf("doc = %+v, err = %v, out = %s", doc, err, out)
	}
	if _, _, err := runQuayAt(t, root, srv, "status", "nope"); err == nil {
		t.Fatal("unknown bundle accepted")
	}
}

func TestQuayInfo(t *testing.T) {
	config := []byte(`{"os":"linux","architecture":"amd64","config":{"Labels":{"org.opencontainers.image.version":"1.2.3-b1","org.opencontainers.image.revision":"deadbeef"}}}`)
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(config))
	manifest := []byte(`{"config":{"digest":"` + configDigest + `"}}`)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
	blobs := map[string][]byte{configDigest: config, digest: manifest}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/auth":
			w.Write([]byte(`{"token":"tok"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/bundledock/bun/"):
			parts := strings.Split(r.URL.Path, "/")
			if data, ok := blobs[parts[len(parts)-1]]; ok {
				w.Write(data)
				return
			}
			http.NotFound(w, r)
		case r.URL.Path == "/api/v1/repository/bundledock/bun/tag/":
			json.NewEncoder(w).Encode(map[string]any{"page": 1, "has_additional": false, "tags": []quayTag{{Name: "1.2.3-b1", ManifestDigest: digest, LastModified: "Fri, 30 Oct 2026 10:00:00 -0000"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	out, _, err := runQuay(t, srv, "info", "bun")
	if err != nil || !strings.Contains(out, "linux/amd64") || !strings.Contains(out, "deadbeef") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	out, _, err = runQuay(t, srv, "info", "bun", "--json")
	var doc struct {
		Schema string `json:"schema"`
		Tags   []struct {
			Tag, Version, Revision string
		} `json:"tags"`
	}
	if err != nil || json.Unmarshal([]byte(out), &doc) != nil || doc.Schema != "bdock.quay.info/v1" || len(doc.Tags) != 1 || doc.Tags[0].Version != "1.2.3-b1" {
		t.Fatalf("doc = %+v, err = %v", doc, err)
	}
	if _, _, err := runQuay(t, srv, "info"); err == nil {
		t.Fatal("bundle argument not required")
	}
}

func TestQuayDescribe(t *testing.T) {
	live := "old"
	token := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository/bundledock/toolbox" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPut {
			token = r.Header.Get("Authorization")
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			live = body["description"]
			w.Write([]byte("{}"))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"description": live, "is_public": true})
	}))
	defer srv.Close()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bundles", "toolbox"), 0o755)
	os.WriteFile(filepath.Join(root, "bundles", "toolbox", "description.md"), []byte("new\n"), 0o644)
	out, _, err := runQuayAt(t, root, srv, "describe", "--json")
	if err == nil || !strings.Contains(out, `"differs"`) || !strings.Contains(out, "bdock.quay.describe/v1") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	t.Setenv("QUAY_TOKEN", "")
	if _, _, err = runQuayAt(t, root, srv, "describe", "--apply"); err == nil {
		t.Fatal("apply without QUAY_TOKEN succeeded")
	}
	t.Setenv("QUAY_TOKEN", "secret-token")
	out, errOut, err := runQuayAt(t, root, srv, "describe", "--apply")
	if err != nil || live != "new" || token != "Bearer secret-token" || strings.Contains(out+errOut, "secret-token") {
		t.Fatalf("apply: err = %v live = %q token set = %v", err, live, token != "")
	}
	if _, _, err = runQuayAt(t, root, srv, "describe"); err != nil {
		t.Fatalf("second check: %v", err)
	}
	if _, _, err = runQuayAt(t, root, srv, "describe", "bun"); err == nil {
		t.Fatal("bundle without description file accepted")
	}
	if _, _, err = runQuayAt(t, root, srv, "describe", "--token", "x"); err == nil {
		t.Fatal("token flag exists")
	}
}

func TestCommittedDescriptions(t *testing.T) {
	for _, name := range []string{"toolbox", "nixbox"} {
		raw, err := os.ReadFile("../../bundles/" + name + "/description.md")
		if err != nil || len([]rune(string(raw))) > 5000 || !strings.Contains(string(raw), "https://wstein.github.io/bundledock/") || !strings.Contains(string(raw), "https://github.com/wstein/bundledock") {
			t.Fatalf("%s description: err = %v", name, err)
		}
	}
}

type stubGitHub struct{ run quay.Run }

func (g stubGitHub) Run(context.Context, string, bool) (quay.Run, error)          { return g.run, nil }
func (g stubGitHub) Attempt(context.Context, string, int, bool) (quay.Run, error) { return g.run, nil }
func (g stubGitHub) Workflow(context.Context, bool) (quay.Workflow, error) {
	return quay.Workflow{ID: 1, Path: g.run.Path, State: "active"}, nil
}
func (g stubGitHub) Jobs(context.Context, string, int, bool) ([]quay.Job, error) {
	return []quay.Job{{ID: 1, RunID: g.run.ID, RunAttempt: g.run.RunAttempt, Status: "completed", Conclusion: "failure"}}, nil
}

func TestQuayPrune(t *testing.T) {
	labels := map[string]string{
		"io.bundledock.bundle": "toolbox", "io.bundledock.github.run-id": "123", "io.bundledock.github.run-attempt": "1",
		"org.opencontainers.image.source": "https://github.com/wstein/bundledock", "org.opencontainers.image.revision": strings.Repeat("a", 40),
		"org.opencontainers.image.version": "2026.10.01-b4.1",
	}
	docs := map[string][]byte{}
	add := func(v any) string {
		raw, _ := json.Marshal(v)
		d := fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
		docs[d] = raw
		return d
	}
	config := add(map[string]any{"os": "linux", "architecture": "amd64", "config": map[string]any{"Labels": labels}})
	digest := add(map[string]any{"config": map[string]any{"digest": config}})
	tags := []quayTag{
		{Name: "staging-2026.10.01-b4.1-amd64", ManifestDigest: digest, LastModified: "Thu, 01 Oct 2026 00:00:00 -0000"},
		{Name: "2026.10.01-b4.1", ManifestDigest: "sha256:" + strings.Repeat("c", 64), LastModified: "Thu, 01 Oct 2026 00:00:00 -0000"},
		{Name: "stable", ManifestDigest: "sha256:" + strings.Repeat("c", 64), LastModified: "Thu, 01 Oct 2026 00:00:00 -0000"},
	}
	var deletes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/v2/auth":
			w.Write([]byte(`{"token":"tok"}`))
		case r.Method == http.MethodDelete:
			deletes = append(deletes, path)
			w.WriteHeader(202)
		case strings.HasPrefix(path, "/v2/bundledock/toolbox/"):
			parts := strings.Split(path, "/")
			if raw, ok := docs[parts[len(parts)-1]]; ok {
				w.Write(raw)
				return
			}
			http.NotFound(w, r)
		case strings.HasSuffix(path, "/tag/"):
			var rows []quayTag
			if strings.Contains(path, "/toolbox/") {
				rows = tags
				if name := r.URL.Query().Get("specificTag"); name != "" {
					rows = nil
					for _, tag := range tags {
						if tag.Name == name {
							rows = append(rows, tag)
						}
					}
				}
			}
			if rows == nil {
				rows = []quayTag{}
			}
			json.NewEncoder(w).Encode(map[string]any{"page": 1, "tags": rows, "has_additional": false})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	run := quay.Run{ID: 123, RunNumber: 4, RunAttempt: 1, Status: "completed", Conclusion: "failure", UpdatedAt: "2026-10-01T00:00:00Z",
		Event: "workflow_dispatch", HeadBranch: "main", HeadSHA: strings.Repeat("a", 40), WorkflowID: 1, Path: ".github/workflows/publish-bundles.yml"}
	run.Repository.FullName, run.HeadRepository.FullName = "wstein/bundledock", "wstein/bundledock"
	defer func(old func() quay.GitHub) { newGitHub = old }(newGitHub)
	newGitHub = func() quay.GitHub { return stubGitHub{run} }

	root := t.TempDir()
	for _, bundle := range []string{"toolbox", "code-server", "node", "bun", "codex", "agy", "claude", "opentofu", "nixbox"} {
		writeRecord(t, root, bundle, "2026.10.30-b3", "sha256:"+strings.Repeat("a", 64))
	}
	planFile := filepath.Join(t.TempDir(), "plan.json")
	t.Setenv("REPOSITORY_USERNAME", "")
	t.Setenv("REPOSITORY_TOKEN", "")

	out, errOut, err := runQuayAt(t, root, srv, "prune", "--json", "--output", planFile)
	if err != nil || len(deletes) != 0 || !strings.Contains(out, "bdock.quay.prune/v1") || !strings.Contains(errOut, "nothing changed") {
		t.Fatalf("dry run: err = %v deletes = %v out = %q", err, deletes, out)
	}
	var plan struct {
		Schema int `json:"schema"`
		Rows   []struct {
			Name     string
			Eligible bool
		}
	}
	raw, _ := os.ReadFile(planFile)
	if json.Unmarshal(raw, &plan) != nil || plan.Schema != 1 || len(plan.Rows) != 1 || !plan.Rows[0].Eligible {
		t.Fatalf("plan = %s", raw)
	}
	if _, _, err = runQuayAt(t, root, srv, "prune", "--apply"); err == nil {
		t.Fatal("apply without a saved plan accepted")
	}
	if _, _, err = runQuayAt(t, root, srv, "prune", "--apply", "--approved-plan", planFile); err == nil || len(deletes) != 0 {
		t.Fatalf("apply without credentials: err = %v deletes = %v", err, deletes)
	}
	t.Setenv("REPOSITORY_USERNAME", "robot")
	t.Setenv("REPOSITORY_TOKEN", "pw")
	result := filepath.Join(t.TempDir(), "result.json")
	out, _, err = runQuayAt(t, root, srv, "prune", "--apply", "--approved-plan", planFile, "--output", result, "--json")
	if err != nil || len(deletes) != 1 || deletes[0] != "/v2/bundledock/toolbox/manifests/staging-2026.10.01-b4.1-amd64" || !strings.Contains(out, `"deleted"`) {
		t.Fatalf("apply: err = %v deletes = %v out = %q", err, deletes, out)
	}
	if saved, _ := os.ReadFile(result); !strings.Contains(string(saved), "deleted") || strings.Contains(string(saved), "pw") {
		t.Fatalf("result = %s", saved)
	}
	if _, _, err = runQuayAt(t, root, srv, "prune", "--failed-run", "123"); err == nil {
		t.Fatal("failed-run without attempt accepted")
	}
}

func TestQuayDescribeInit(t *testing.T) {
	// No network: a server that fails every request proves none is made.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
		http.Error(w, "no", 500)
	}))
	defer srv.Close()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.MkdirAll(filepath.Join(root, "bundles", "bun"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "bun.md"), []byte("---\ndescription: Bun runtime and bunx.\n---\n"), 0o644)
	out, _, err := runQuayAt(t, root, srv, "describe", "--init", "bun", "--json")
	var doc struct {
		Schema  string `json:"schema"`
		Path    string `json:"path"`
		Written bool   `json:"written"`
	}
	if err != nil || json.Unmarshal([]byte(out), &doc) != nil || !doc.Written || doc.Schema != "bdock.quay.describe-init/v1" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "bundles", "bun", "description.md"))
	if !strings.Contains(string(raw), "Bun runtime and bunx.") {
		t.Fatalf("draft = %s", raw)
	}
	if _, _, err = runQuayAt(t, root, srv, "describe", "--init", "bun"); err == nil {
		t.Fatal("overwrote an existing description")
	}
	if _, _, err = runQuayAt(t, root, srv, "describe", "--init"); err == nil {
		t.Fatal("--init without a bundle accepted")
	}
	if _, _, err = runQuayAt(t, root, srv, "describe", "--init", "--apply", "bun"); err == nil {
		t.Fatal("--init with --apply accepted")
	}
}

type rewriteTransport struct{ target *url.URL }

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host, req.Host = r.target.Scheme, r.target.Host, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestQuayHasNoEndpointFlag(t *testing.T) {
	srv := fakeQuayAPI(t, nil)
	if _, _, err := runQuayAt(t, "../..", srv, "list", "--api-url", srv.URL); err == nil {
		t.Fatal("--api-url exists; credentials could be sent to any host")
	}
}

type stubGit struct {
	verified []string
	found    bool
}

func (g *stubGit) VerifySource(_ context.Context, commit string) error {
	g.verified = append(g.verified, commit)
	return nil
}
func (g *stubGit) RemoteTag(context.Context, string, string, string) (bool, error) {
	return g.found, nil
}

func TestQuayPrunePublication(t *testing.T) {
	commit := strings.Repeat("a", 40)
	labels := map[string]string{
		"io.bundledock.bundle": "toolbox", "io.bundledock.github.run-id": "123", "io.bundledock.github.run-attempt": "1",
		"org.opencontainers.image.source": "https://github.com/wstein/bundledock", "org.opencontainers.image.revision": commit,
		"org.opencontainers.image.version": "2026.10.01-b4.1",
	}
	docs := map[string][]byte{}
	add := func(v any) string {
		raw, _ := json.Marshal(v)
		d := fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
		docs[d] = raw
		return d
	}
	manifest := func(arch string) string {
		config := add(map[string]any{"os": "linux", "architecture": arch, "config": map[string]any{"Labels": labels}})
		return add(map[string]any{"config": map[string]any{"digest": config}})
	}
	amd, arm := manifest("amd64"), manifest("arm64")
	index := map[string]any{"manifests": []any{
		map[string]any{"digest": amd, "platform": map[string]any{"os": "linux", "architecture": "amd64"}},
		map[string]any{"digest": arm, "platform": map[string]any{"os": "linux", "architecture": "arm64"}},
	}}
	indexDigest := add(index)
	for _, ref := range []string{"2026.10.01-b4.1", "stable", "latest"} {
		docs[ref] = docs[indexDigest]
	}
	const modified = "Thu, 01 Oct 2026 00:00:00 -0000"
	tags := []quayTag{
		{Name: "staging-2026.10.01-b4.1-amd64", ManifestDigest: amd, LastModified: modified},
		{Name: "staging-2026.10.01-b4.1-arm64", ManifestDigest: arm, LastModified: modified},
		{Name: "staging-2026.10.01-b9.1-amd64", ManifestDigest: amd, LastModified: modified},
		{Name: "stable", ManifestDigest: indexDigest, LastModified: modified},
	}
	var deletes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/v2/auth":
			w.Write([]byte(`{"token":"tok"}`))
		case r.Method == http.MethodDelete:
			deletes = append(deletes, path)
			w.WriteHeader(202)
		case strings.HasPrefix(path, "/v2/bundledock/toolbox/"):
			parts := strings.Split(path, "/")
			if raw, ok := docs[parts[len(parts)-1]]; ok {
				w.Write(raw)
				return
			}
			http.NotFound(w, r)
		case strings.HasSuffix(path, "/tag/"):
			rows := []quayTag{}
			if strings.Contains(path, "/toolbox/") {
				for _, tag := range tags {
					if name := r.URL.Query().Get("specificTag"); name == "" || tag.Name == name {
						rows = append(rows, tag)
					}
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"page": 1, "tags": rows, "has_additional": false})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	run := quay.Run{ID: 123, RunNumber: 4, RunAttempt: 1, Status: "in_progress", UpdatedAt: "2026-10-14T00:00:00Z",
		Event: "workflow_dispatch", HeadBranch: "main", HeadSHA: commit, WorkflowID: 1, Path: ".github/workflows/publish-bundles.yml"}
	run.Repository.FullName, run.HeadRepository.FullName = "wstein/bundledock", "wstein/bundledock"
	git := &stubGit{found: true}
	defer func(a func() quay.GitHub, b func() quay.Git) { newGitHub, newGit = a, b }(newGitHub, newGit)
	newGitHub = func() quay.GitHub { return stubGitHub{run} }
	newGit = func() quay.Git { return git }

	dir := t.TempDir()
	platform := func(arch, digest string) map[string]any {
		return map[string]any{"architecture": arch, "digest": digest, "image_id": "sha256:" + strings.Repeat("2", 64), "smoked": true,
			"image": "quay.io/bundledock/toolbox", "tag": "2026.10.01-b4.1", "commit": commit, "source": "https://github.com/wstein/bundledock",
			"created": "2026-10-01T00:00:00Z", "lock_sha256": strings.Repeat("1", 64), "run_id": "123"}
	}
	receipt, _ := json.Marshal(map[string]any{
		"image": "quay.io/bundledock/toolbox", "tag": "2026.10.01-b4.1", "commit": commit, "source": "https://github.com/wstein/bundledock",
		"created": "2026-10-01T00:00:00Z", "lock_sha256": strings.Repeat("1", 64), "run_id": "123", "digest": indexDigest,
		"reference": "quay.io/bundledock/toolbox@" + indexDigest, "platforms": []any{platform("amd64", amd), platform("arm64", arm)},
	})
	receiptFile := filepath.Join(dir, "publication.json")
	os.WriteFile(receiptFile, receipt, 0o644)
	t.Setenv("PUBLICATION_SHA", commit)
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("REPOSITORY_USERNAME", "")
	t.Setenv("REPOSITORY_TOKEN", "")
	root := t.TempDir()
	planFile := filepath.Join(dir, "plan.json")

	// Selector errors stop before any evidence is read.
	for name, args := range map[string][]string{
		"publication without bundle": {"prune", "--publication", receiptFile},
		"bundle without publication": {"prune", "--bundle", "toolbox"},
		"both modes":                 {"prune", "--publication", receiptFile, "--bundle", "toolbox", "--failed-run", "123", "--failed-attempt", "1"},
		"unknown bundle":             {"prune", "--publication", receiptFile, "--bundle", "nope"},
		"missing receipt":            {"prune", "--publication", filepath.Join(dir, "none.json"), "--bundle", "toolbox"},
	} {
		if _, _, err := runQuayAt(t, root, srv, args...); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if len(git.verified) != 0 {
		t.Fatalf("source verified for an invalid selection: %v", git.verified)
	}

	_, _, err := runQuayAt(t, root, srv, "prune", "--publication", receiptFile, "--bundle", "toolbox", "--output", planFile)
	if err != nil || len(deletes) != 0 || len(git.verified) != 1 {
		t.Fatalf("dry run: err = %v deletes = %v verified = %v", err, deletes, git.verified)
	}
	var plan struct {
		Mode   string          `json:"mode"`
		Target json.RawMessage `json:"target"`
		Rows   []struct {
			Name     string
			Eligible bool
		}
	}
	raw, _ := os.ReadFile(planFile)
	if json.Unmarshal(raw, &plan) != nil || plan.Mode != "publication" || plan.Target != nil || len(plan.Rows) != 2 || !plan.Rows[0].Eligible || !plan.Rows[1].Eligible {
		t.Fatalf("plan = %s", raw)
	}
	args := []string{"prune", "--publication", receiptFile, "--bundle", "toolbox", "--apply", "--approved-plan", planFile}
	if _, _, err = runQuayAt(t, root, srv, args...); err == nil || len(deletes) != 0 {
		t.Fatalf("apply without credentials: err = %v deletes = %v", err, deletes)
	}
	// A fallback request does not accept the publication plan.
	t.Setenv("REPOSITORY_USERNAME", "robot")
	t.Setenv("REPOSITORY_TOKEN", "pw")
	if _, _, err = runQuayAt(t, root, srv, "prune", "--apply", "--approved-plan", planFile); err == nil || len(deletes) != 0 {
		t.Fatalf("fallback request applied a publication plan: err = %v deletes = %v", err, deletes)
	}
	result := filepath.Join(dir, "result.json")
	_, _, err = runQuayAt(t, root, srv, append(args, "--output", result)...)
	if err != nil || len(deletes) != 2 || deletes[0] != "/v2/bundledock/toolbox/manifests/staging-2026.10.01-b4.1-amd64" || deletes[1] != "/v2/bundledock/toolbox/manifests/staging-2026.10.01-b4.1-arm64" {
		t.Fatalf("apply: err = %v deletes = %v", err, deletes)
	}
	if saved, _ := os.ReadFile(result); !strings.Contains(string(saved), `"mode": "publication"`) || strings.Contains(string(saved), "pw") {
		t.Fatalf("result = %s", saved)
	}
	// The receipt must be verified again by the apply step: a missing source tag stops it.
	git.found = false
	deletes = nil
	if _, _, err = runQuayAt(t, root, srv, args...); err == nil || len(deletes) != 0 {
		t.Fatalf("apply without the source tag: err = %v deletes = %v", err, deletes)
	}
}

func TestQuayPruneWritesTheStepSummary(t *testing.T) {
	srv := fakeQuayAPI(t, nil)
	run := quay.Run{}
	defer func(old func() quay.GitHub) { newGitHub = old }(newGitHub)
	newGitHub = func() quay.GitHub { return stubGitHub{run} }
	t.Setenv("REPOSITORY_USERNAME", "robot")
	t.Setenv("REPOSITORY_TOKEN", "pw-secret")
	root := t.TempDir()
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.json")

	// Without GITHUB_STEP_SUMMARY nothing is written.
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	if _, _, err := runQuayAt(t, root, srv, "prune", "--repository", "toolbox", "--output", plan); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	if _, err := os.Stat(summary); err == nil {
		t.Fatal("summary created without a write")
	}
	os.WriteFile(summary, []byte("earlier step\n"), 0o644)
	if _, _, err := runQuayAt(t, root, srv, "prune", "--repository", "toolbox", "--output", plan); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runQuayAt(t, root, srv, "prune", "--repository", "toolbox", "--apply", "--approved-plan", plan, "--output", filepath.Join(dir, "result.json")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(summary)
	got := string(raw)
	if !strings.HasPrefix(got, "earlier step\n") || !strings.Contains(got, "### Staging cleanup (fallback)") || !strings.Contains(got, "### Staging cleanup apply (fallback)") ||
		!strings.Contains(got, "Staging tags: 0; eligible: 0") || strings.Contains(got, "pw-secret") || strings.Contains(got, "robot") {
		t.Fatalf("summary = %q", got)
	}
	// An unwritable summary is a warning, not a failed cleanup.
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(dir, "none", "summary.md"))
	_, errOut, err := runQuayAt(t, root, srv, "prune", "--repository", "toolbox")
	if err != nil || !strings.Contains(errOut, "step summary") {
		t.Fatalf("err = %v stderr = %q", err, errOut)
	}
}
