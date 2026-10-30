package quay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var pruneNow = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

type fakeEvidence map[string][]byte

func (f fakeEvidence) Manifest(_ context.Context, d string) ([]byte, error) { return f.get(d) }
func (f fakeEvidence) Blob(_ context.Context, d string) ([]byte, error)     { return f.get(d) }
func (f fakeEvidence) get(d string) ([]byte, error) {
	if raw, ok := f[d]; ok {
		return raw, nil
	}
	return nil, &StatusError{Code: 404}
}

type fakeGitHub struct {
	run, attempt Run
	workflow     Workflow
	jobs         []Job
	fail         bool
}

func (g *fakeGitHub) Run(context.Context, string, bool) (Run, error) {
	if g.fail {
		return Run{}, fmt.Errorf("GitHub evidence unavailable")
	}
	return g.run, nil
}
func (g *fakeGitHub) Attempt(context.Context, string, int, bool) (Run, error) { return g.attempt, nil }
func (g *fakeGitHub) Workflow(context.Context, bool) (Workflow, error)        { return g.workflow, nil }
func (g *fakeGitHub) Jobs(context.Context, string, int, bool) ([]Job, error)  { return g.jobs, nil }

type fakeStore struct {
	rows    []Tag
	deleted []string
}

func (s *fakeStore) Tags(_ context.Context, _, name string) ([]Tag, error) {
	var out []Tag
	for _, r := range s.rows {
		if name == "" || r.Name == name {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *fakeStore) Delete(_ context.Context, repo, name string) error {
	s.deleted = append(s.deleted, repo+"/"+name)
	return nil
}

type pruneFixture struct {
	t        *testing.T
	docs     fakeEvidence
	labels   map[string]string
	row      Tag
	gh       *fakeGitHub
	store    *fakeStore
	failed   *FailedRun
	pruner   *Pruner
	repo     string
	arch     string
	versionT string
}

func jsonDoc(t *testing.T, docs fakeEvidence, v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	docs[digest] = raw
	return digest
}

func newPruneFixture(t *testing.T) *pruneFixture {
	f := &pruneFixture{t: t, docs: fakeEvidence{}, repo: "toolbox", arch: "amd64"}
	f.labels = map[string]string{
		"io.bundledock.bundle":              "toolbox",
		"io.bundledock.github.run-id":       "123",
		"io.bundledock.github.run-attempt":  "1",
		"org.opencontainers.image.source":   SourceURL,
		"org.opencontainers.image.revision": strings.Repeat("a", 40),
		"org.opencontainers.image.version":  "2026.10.01-b4.1",
	}
	f.setRow("staging-2026.10.01-b4.1-amd64")
	f.gh = &fakeGitHub{}
	f.gh.run = Run{ID: 123, RunNumber: 4, RunAttempt: 1, Status: "completed", Conclusion: "failure", UpdatedAt: "2026-10-01T00:00:00Z",
		Event: "workflow_dispatch", HeadBranch: "main", HeadSHA: strings.Repeat("a", 40), WorkflowID: 456, Path: ".github/workflows/publish-bundles.yml"}
	f.gh.run.Repository.FullName, f.gh.run.HeadRepository.FullName = GitHubRepository, GitHubRepository
	f.gh.attempt = f.gh.run
	f.gh.workflow = Workflow{ID: 456, Path: ".github/workflows/publish-bundles.yml", State: "active"}
	f.gh.jobs = []Job{{ID: 789, RunID: 123, RunAttempt: 1, Status: "completed", Conclusion: "failure"}}
	f.store = &fakeStore{rows: []Tag{f.row}}
	f.pruner = &Pruner{Store: f.store, GitHub: f.gh, Now: func() time.Time { return pruneNow }, Repos: []string{"toolbox"},
		Registry: func(context.Context, string) (Evidence, error) { return f.docs, nil }}
	return f
}

// setRow (re)builds the registry documents for the current labels.
func (f *pruneFixture) setRow(name string) {
	config := jsonDoc(f.t, f.docs, map[string]any{"os": "linux", "architecture": "amd64", "config": map[string]any{"Labels": f.labels}})
	digest := jsonDoc(f.t, f.docs, map[string]any{"config": map[string]any{"digest": config}})
	f.row = Tag{Name: name, ManifestDigest: digest, LastModified: "Thu, 01 Oct 2026 00:00:00 -0000"}
}

func (f *pruneFixture) assess() PlanRow {
	return f.pruner.Assess(context.Background(), f.repo, f.row, false)
}

func TestCompletedOldStagingIsEligible(t *testing.T) {
	f := newPruneFixture(t)
	if row := f.assess(); !row.Eligible || row.Bundle != "toolbox" || row.Architecture != "amd64" {
		t.Fatalf("row = %+v", row)
	}
}

func TestDailyRevisionUsesAttemptLabels(t *testing.T) {
	f := newPruneFixture(t)
	f.labels["org.opencontainers.image.version"] = "2026.10.01-b1"
	f.setRow("staging-2026.10.01-b1-amd64")
	f.gh.run.RunNumber = 98
	if !f.assess().Eligible {
		t.Fatalf("daily tag ineligible: %+v", f.assess())
	}
	f.gh.run.RunAttempt = 2
	f.gh.run.UpdatedAt = "2026-10-14T00:00:00Z"
	if f.assess().Eligible {
		t.Fatal("a recent rerun is eligible")
	}
}

func TestSevenDayBoundaryAndActiveRuns(t *testing.T) {
	for name, mutate := range map[string]func(*Run){
		"in progress": func(r *Run) { r.Status = "in_progress" },
		"queued":      func(r *Run) { r.Status = "queued" },
		"recent":      func(r *Run) { r.UpdatedAt = "2026-10-13T00:00:00Z" },
		"attempt 0":   func(r *Run) { r.RunAttempt = 0 },
		"run number":  func(r *Run) { r.RunNumber = 5 },
		"foreign":     func(r *Run) { r.Path = ".github/workflows/ci.yml" },
	} {
		f := newPruneFixture(t)
		mutate(&f.gh.run)
		if f.assess().Eligible {
			t.Errorf("%s: eligible", name)
		}
	}
	f := newPruneFixture(t)
	f.row.LastModified = "Wed, 07 Oct 2026 00:00:00 -0000"
	if !f.assess().Eligible {
		t.Fatal("exactly seven days old must be eligible")
	}
	f.row.LastModified = "Wed, 07 Oct 2026 00:00:01 -0000"
	if f.assess().Eligible {
		t.Fatal("younger than seven days must be retained")
	}
}

func TestProtectedNamesIndexesAndImmutableAreRetained(t *testing.T) {
	f := newPruneFixture(t)
	for name, mutate := range map[string]func(*Tag){
		"stable":     func(r *Tag) { r.Name = "stable" },
		"latest":     func(r *Tag) { r.Name = "latest" },
		"version":    func(r *Tag) { r.Name = "2026.10.01-b4.1" },
		"wrong arch": func(r *Tag) { r.Name = "staging-2026.10.01-b4.1-arm64" },
		"index":      func(r *Tag) { r.IsManifestList = true },
		"immutable":  func(r *Tag) { r.Immutable = true },
		"digest":     func(r *Tag) { r.ManifestDigest = "sha256:" + strings.Repeat("f", 64) },
	} {
		row := f.row
		mutate(&row)
		if f.pruner.Assess(context.Background(), "toolbox", row, false).Eligible {
			t.Errorf("%s: eligible", name)
		}
	}
	if f.pruner.Assess(context.Background(), "foreign/toolbox", f.row, false).Eligible {
		t.Error("foreign repository eligible")
	}
}

func TestMissingEvidenceFailsClosed(t *testing.T) {
	f := newPruneFixture(t)
	f.gh.fail = true
	row := f.assess()
	if row.Eligible || strings.Contains(row.Reason, "GitHub") {
		t.Fatalf("row = %+v", row)
	}
	f = newPruneFixture(t)
	f.gh.run = Run{}
	if f.assess().Eligible {
		t.Fatal("empty run eligible")
	}
}

func TestLabelMismatchesAreRetained(t *testing.T) {
	for key, value := range map[string]string{
		"io.bundledock.bundle":              "nixbox",
		"org.opencontainers.image.source":   "https://example.com/x",
		"org.opencontainers.image.revision": strings.Repeat("b", 40),
		"io.bundledock.github.run-id":       "999",
	} {
		f := newPruneFixture(t)
		f.labels[key] = value
		f.setRow(f.row.Name)
		if f.assess().Eligible {
			t.Errorf("%s=%s: eligible", key, value)
		}
	}
}

func plan(f *pruneFixture) Plan {
	return Plan{Schema: 1, Mode: ModeFallback, Rows: []PlanRow{f.assess()}}
}

func TestPlanNeverDeletesAndApplyRechecks(t *testing.T) {
	f := newPruneFixture(t)
	f.store.rows = append(f.store.rows, Tag{Name: "stable"}, Tag{Name: "2026.10.01-b4.1"})
	got, err := f.pruner.Plan(context.Background())
	if err != nil || len(got.Rows) != 1 || !got.Rows[0].Eligible || len(f.store.deleted) != 0 {
		t.Fatalf("plan = %+v, err = %v, deleted = %v", got, err, f.store.deleted)
	}
	approved := plan(f)
	f.gh.run.Status = "in_progress"
	if _, err := f.pruner.Apply(context.Background(), approved, nil); err == nil || len(f.store.deleted) != 0 {
		t.Fatalf("active run: err = %v deleted = %v", err, f.store.deleted)
	}
	f.gh.run.Status = "completed"
	f.store.rows[0].ManifestDigest = "sha256:" + strings.Repeat("f", 64)
	if _, err := f.pruner.Apply(context.Background(), approved, nil); err == nil || len(f.store.deleted) != 0 {
		t.Fatalf("changed digest: err = %v deleted = %v", err, f.store.deleted)
	}
}

func TestApplyDeletesOnlyExactTagAndIsIdempotent(t *testing.T) {
	f := newPruneFixture(t)
	approved := plan(f)
	var reports [][]Action
	actions, err := f.pruner.Apply(context.Background(), approved, func(a []Action) { reports = append(reports, append([]Action{}, a...)) })
	if err != nil || len(actions) != 1 || actions[0].Action != "deleted" || reports[0][0].Action != "deleted" {
		t.Fatalf("actions = %v, err = %v", actions, err)
	}
	if len(f.store.deleted) != 1 || f.store.deleted[0] != "toolbox/"+f.row.Name {
		t.Fatalf("deleted = %v", f.store.deleted)
	}
	f.store.rows = nil
	actions, err = f.pruner.Apply(context.Background(), approved, nil)
	if err != nil || actions[0].Action != "already absent" || len(f.store.deleted) != 1 {
		t.Fatalf("second apply: %v %v", actions, err)
	}
}

func TestApplyValidatesEveryApprovedRowBeforeDeleting(t *testing.T) {
	f := newPruneFixture(t)
	good := f.assess()
	for name, bad := range map[string]PlanRow{
		"alias":     {Repository: "toolbox", Name: "stable", Eligible: true, ManifestDigest: good.ManifestDigest},
		"digest":    {Repository: "toolbox", Name: good.Name + "x", Eligible: true, ManifestDigest: "nope"},
		"duplicate": good,
		"scope":     {Repository: "bun", Name: good.Name, Eligible: true, ManifestDigest: good.ManifestDigest},
	} {
		f.store.deleted = nil
		approved := Plan{Schema: 1, Mode: ModeFallback, Rows: []PlanRow{good, bad}}
		if _, err := f.pruner.Apply(context.Background(), approved, nil); err == nil || len(f.store.deleted) != 0 {
			t.Errorf("%s: err = %v deleted = %v", name, err, f.store.deleted)
		}
	}
	if _, err := f.pruner.Apply(context.Background(), Plan{Schema: 2, Mode: ModeFallback}, nil); err == nil {
		t.Error("invalid schema accepted")
	}
	if _, err := f.pruner.Apply(context.Background(), Plan{Schema: 1, Mode: "publication"}, nil); err == nil {
		t.Error("unsupported mode accepted")
	}
}

func failedFixture(t *testing.T) *pruneFixture {
	f := newPruneFixture(t)
	f.failed = &FailedRun{RunID: "123", Attempt: 1}
	f.pruner.FailedRun = f.failed
	f.gh.run.UpdatedAt = "2026-10-13T00:00:00Z"
	f.gh.attempt = f.gh.run
	f.row.LastModified = "Tue, 13 Oct 2026 00:00:00 -0000"
	return f
}

func TestFailedRunRecentTagsEligibleOnlyForExactAttempt(t *testing.T) {
	f := failedFixture(t)
	if row := f.assess(); !row.Eligible || !strings.Contains(row.Reason, "failed publication") {
		t.Fatalf("row = %+v", row)
	}
	for name, mutate := range map[string]func(*pruneFixture){
		"unavailable":     func(f *pruneFixture) { f.gh.fail = true },
		"active":          func(f *pruneFixture) { f.gh.run.Status = "in_progress"; f.gh.attempt = f.gh.run },
		"successful":      func(f *pruneFixture) { f.gh.run.Conclusion = "success"; f.gh.attempt = f.gh.run },
		"foreign":         func(f *pruneFixture) { f.gh.run.Repository.FullName = "evil/x"; f.gh.attempt = f.gh.run },
		"rerun":           func(f *pruneFixture) { f.gh.run.RunAttempt = 2; f.gh.attempt = f.gh.run },
		"other run":       func(f *pruneFixture) { f.failed.RunID = "124" },
		"other attempt":   func(f *pruneFixture) { f.failed.Attempt = 2 },
		"workflow":        func(f *pruneFixture) { f.gh.workflow.State = "disabled_manually" },
		"attempt differs": func(f *pruneFixture) { f.gh.attempt.Conclusion = "cancelled" },
		"no jobs":         func(f *pruneFixture) { f.gh.jobs = nil },
		"active job":      func(f *pruneFixture) { f.gh.jobs[0].Status = "in_progress" },
		"other job":       func(f *pruneFixture) { f.gh.jobs[0].RunAttempt = 2 },
		"duplicate job":   func(f *pruneFixture) { f.gh.jobs = append(f.gh.jobs, f.gh.jobs[0]) },
	} {
		f := failedFixture(t)
		mutate(f)
		if f.assess().Eligible {
			t.Errorf("%s: eligible", name)
		}
	}
}

func TestFailedRunPlanScopeIsChecked(t *testing.T) {
	f := failedFixture(t)
	approved := Plan{Schema: 1, Mode: ModeFailedRun, Target: &FailedRun{RunID: "123", Attempt: 1}, Rows: []PlanRow{f.assess()}}
	f.store.rows = []Tag{f.row}
	actions, err := f.pruner.Apply(context.Background(), approved, nil)
	if err != nil || actions[0].Action != "deleted" {
		t.Fatalf("actions = %v err = %v", actions, err)
	}
	approved.Target = &FailedRun{RunID: "124", Attempt: 1}
	if _, err := f.pruner.Apply(context.Background(), approved, nil); err == nil {
		t.Fatal("plan target differs from request accepted")
	}
	fallback := Plan{Schema: 1, Mode: ModeFallback, Rows: approved.Rows}
	if _, err := f.pruner.Apply(context.Background(), fallback, nil); err == nil {
		t.Fatal("plan mode differs from request accepted")
	}
	// A run that was rerun after the dry run is rejected by the plan's own proof.
	f.gh.run.RunAttempt = 2
	f.gh.attempt = f.gh.run
	f.store.rows = []Tag{f.row}
	approved.Target = &FailedRun{RunID: "123", Attempt: 1}
	if _, err := f.pruner.Apply(context.Background(), approved, nil); err == nil {
		t.Fatal("rerun accepted")
	}
}

// A rerun that starts after the jobs were read must invalidate the proof: only
// the final fresh read of the run can see it.
func TestFailedRunProofResamplesRunAfterJobs(t *testing.T) {
	f := failedFixture(t)
	g := &rerunAfterJobs{fakeGitHub: f.gh}
	if _, err := ProveFailedRun(context.Background(), g, "123", 1, true, nil); err == nil || !strings.Contains(err.Error(), "changed while checking") {
		t.Fatalf("a run that changed after the jobs read was accepted: %v", err)
	}
	if g.runs != 2 {
		t.Fatalf("run reads = %d, want the initial read and the resample", g.runs)
	}
	// Unchanged evidence passes with the same two reads.
	g = &rerunAfterJobs{fakeGitHub: f.gh, quiet: true}
	if _, err := ProveFailedRun(context.Background(), g, "123", 1, true, nil); err != nil || g.runs != 2 {
		t.Fatalf("err = %v, run reads = %d", err, g.runs)
	}
}

type rerunAfterJobs struct {
	*fakeGitHub
	quiet    bool
	jobsSeen bool
	runs     int
}

func (g *rerunAfterJobs) Jobs(ctx context.Context, id string, a int, fresh bool) ([]Job, error) {
	g.jobsSeen = true
	return g.fakeGitHub.Jobs(ctx, id, a, fresh)
}
func (g *rerunAfterJobs) Run(ctx context.Context, id string, fresh bool) (Run, error) {
	g.runs++
	run, err := g.fakeGitHub.Run(ctx, id, fresh)
	if g.jobsSeen && !g.quiet {
		run.RunAttempt++
	}
	return run, err
}

func TestNodeCurrentLabelsRequireBundle(t *testing.T) {
	f := newPruneFixture(t)
	f.repo = "node"
	f.pruner.Repos = []string{"node"}
	f.labels["io.bundledock.bundle"] = "node-current"
	f.labels["org.opencontainers.image.version"] = "26.12.0-b1"
	f.labels["io.bundledock.github.run-attempt"] = "1"
	f.setRow("staging-26.12.0-b1-amd64")
	f.gh.run.RunNumber = 4
	if row := f.assess(); !row.Eligible || row.Bundle != "node-current" {
		t.Fatalf("row = %+v", row)
	}
	delete(f.labels, "io.bundledock.bundle")
	f.setRow("staging-26.12.0-b1-amd64")
	if f.assess().Eligible {
		t.Fatal("unlabeled node-current staging tag eligible")
	}
}

// --- transport -------------------------------------------------------------

func TestDeleteTagUsesRobotOnlyForRegistryAndOnlyStagingTags(t *testing.T) {
	var deletes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/auth":
			if user, pass, ok := r.BasicAuth(); !ok || user != "robot" || pass != "pw" {
				w.WriteHeader(401)
				return
			}
			if !strings.Contains(r.URL.Query().Get("scope"), "pull,push") {
				t.Errorf("scope = %s", r.URL.Query().Get("scope"))
			}
			w.Write([]byte(`{"token":"tok"}`))
		case r.Method == http.MethodDelete:
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Error("delete without registry token")
			}
			deletes = append(deletes, r.URL.Path)
			if strings.Contains(r.URL.Path, "gone") {
				w.WriteHeader(404)
				return
			}
			w.WriteHeader(202)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	ctx := context.Background()
	if err := c.DeleteTag(ctx, "toolbox", "staging-2026.10.01-b4.1-amd64", "robot", "pw"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteTag(ctx, "toolbox", "staging-2026.10.01-b9-amd64", "robot", "pw"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][2]string{{"foreign/toolbox", "staging-2026.10.01-b4.1-amd64"}, {"toolbox", "stable"}, {"toolbox", "../manifest"}, {"toolbox", "staging-x-amd64"}, {"toolbox", "sha256:" + strings.Repeat("a", 64)}} {
		if err := c.DeleteTag(ctx, args[0], args[1], "robot", "pw"); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	if err := c.DeleteTag(ctx, "toolbox", "staging-2026.10.01-b4.1-amd64", "", ""); err == nil {
		t.Error("deletion without credentials accepted")
	}
	if len(deletes) != 2 || deletes[0] != "/v2/bundledock/toolbox/manifests/staging-2026.10.01-b4.1-amd64" {
		t.Fatalf("deletes = %v", deletes)
	}
}

func TestStoreMissingUnpublishedRepositoryIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	store := Store{Client: New(srv.Client(), srv.URL), Unpublished: func(r string) bool { return r == "nixbox" }}
	if tags, err := store.Tags(context.Background(), "nixbox", ""); err != nil || len(tags) != 0 {
		t.Fatalf("tags = %v err = %v", tags, err)
	}
	if _, err := store.Tags(context.Background(), "toolbox", ""); err == nil {
		t.Fatal("missing published repository accepted")
	}
}

func TestGitHubJobsPagination(t *testing.T) {
	pages := map[string]string{
		"actions/runs/1/attempts/1/jobs?per_page=100&page=1": `{"total_count":2,"jobs":[{"id":1}]}`,
		"actions/runs/1/attempts/1/jobs?per_page=100&page=2": `{"total_count":2,"jobs":[{"id":2}]}`,
	}
	gh := &GitHubAPI{Fetch: func(_ context.Context, path string) ([]byte, error) { return []byte(pages[path]), nil }}
	jobs, err := gh.Jobs(context.Background(), "1", 1, false)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs = %v err = %v", jobs, err)
	}
	pages["actions/runs/1/attempts/1/jobs?per_page=100&page=2"] = `{"total_count":2,"jobs":[{"id":1}]}`
	if _, err := (&GitHubAPI{Fetch: gh.Fetch}).Jobs(context.Background(), "1", 1, false); err == nil {
		t.Fatal("duplicate job accepted")
	}
	pages["actions/runs/1/attempts/1/jobs?per_page=100&page=2"] = `{"total_count":3,"jobs":[{"id":2}]}`
	if _, err := (&GitHubAPI{Fetch: gh.Fetch}).Jobs(context.Background(), "1", 1, false); err == nil {
		t.Fatal("changed total accepted")
	}
	if _, err := gh.Run(context.Background(), "../x", false); err == nil {
		t.Fatal("invalid run ID accepted")
	}
}
