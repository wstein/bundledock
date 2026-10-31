package quay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGit scripts the source checks of a publication proof.
type fakeGit struct {
	sourceErr  error
	tagFound   bool
	tagErr     error
	verified   []string
	tagName    string
	annotation string
}

func (g *fakeGit) VerifySource(_ context.Context, commit string) error {
	g.verified = append(g.verified, commit)
	return g.sourceErr
}

func (g *fakeGit) RemoteTag(_ context.Context, name, commit, annotation string) (bool, error) {
	g.tagName, g.annotation = name, annotation
	return g.tagFound, g.tagErr
}

// pubFixture is a published toolbox release 2026.10.01-b4.1 whose amd64
// staging tag is the fallback fixture's row.
type pubFixture struct {
	*pruneFixture
	receipt map[string]any
	index   string
	arm     string
	git     *fakeGit
	env     PublicationEnv
}

const pubTag = "2026.10.01-b4.1"

func newPubFixture(t *testing.T) *pubFixture {
	f := &pubFixture{pruneFixture: newPruneFixture(t), git: &fakeGit{tagFound: true}}
	f.env = PublicationEnv{RunID: "123", RunAttempt: "1", SHA: strings.Repeat("a", 40)}
	armConfig := jsonDoc(t, f.docs, map[string]any{"os": "linux", "architecture": "arm64", "config": map[string]any{"Labels": f.labels}})
	f.arm = jsonDoc(t, f.docs, map[string]any{"config": map[string]any{"digest": armConfig}})
	f.setIndex(f.row.ManifestDigest, f.arm)
	f.receipt = map[string]any{
		"image": "quay.io/bundledock/toolbox", "tag": pubTag, "commit": strings.Repeat("a", 40), "source": SourceURL,
		"created": "2026-10-01T00:00:00Z", "lock_sha256": strings.Repeat("1", 64), "run_id": "123",
		"digest": f.index, "reference": "quay.io/bundledock/toolbox@" + f.index,
	}
	f.setPlatforms()
	f.pruner.Git = f.git
	return f
}

func (f *pubFixture) setIndex(amd, arm string) {
	index := map[string]any{"manifests": []any{
		map[string]any{"digest": amd, "platform": map[string]any{"os": "linux", "architecture": "amd64"}},
		map[string]any{"digest": arm, "platform": map[string]any{"os": "linux", "architecture": "arm64"}},
	}}
	raw, _ := json.Marshal(index)
	f.index = jsonDoc(f.t, f.docs, index)
	for _, ref := range []string{pubTag, "stable", "latest"} {
		f.docs[ref] = raw
	}
}

func (f *pubFixture) setPlatforms() {
	platform := func(arch, digest string) map[string]any {
		return map[string]any{"architecture": arch, "digest": digest, "image_id": "sha256:" + strings.Repeat("2", 64), "smoked": true,
			"image": f.receipt["image"], "tag": f.receipt["tag"], "commit": f.receipt["commit"], "source": f.receipt["source"],
			"created": f.receipt["created"], "lock_sha256": f.receipt["lock_sha256"], "run_id": f.receipt["run_id"]}
	}
	f.receipt["platforms"] = []any{platform("amd64", f.row.ManifestDigest), platform("arm64", f.arm)}
}

func (f *pubFixture) prove() (*Publication, error) {
	raw, _ := json.Marshal(f.receipt)
	return f.pruner.ProvePublication(context.Background(), raw, "toolbox", f.env)
}

func (f *pubFixture) publication() *Publication {
	f.t.Helper()
	p, err := f.prove()
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func TestProvePublicationAcceptsTheVerifiedReceipt(t *testing.T) {
	f := newPubFixture(t)
	p, err := f.prove()
	if err != nil {
		t.Fatal(err)
	}
	if p.Bundle != "toolbox" || p.Tag != pubTag || p.Attempt != 1 || p.RunID != "123" || p.Platforms["amd64"] != f.row.ManifestDigest || p.Platforms["arm64"] != f.arm {
		t.Fatalf("publication = %+v", p)
	}
	if len(f.git.verified) != 1 || f.git.verified[0] != strings.Repeat("a", 40) || f.git.tagName != "toolbox/"+pubTag {
		t.Fatalf("git = %+v", f.git)
	}
	want := "BundleDock toolbox/" + pubTag + "\n\nImage: quay.io/bundledock/toolbox:" + pubTag + "@" + f.index + "\nIndex-Digest: " + f.index +
		"\nSource: " + SourceURL + "\nRevision: " + strings.Repeat("a", 40) + "\nWorkflow: " + SourceURL + "/actions/runs/123/attempts/1\n"
	if f.git.annotation != want {
		t.Fatalf("annotation = %q", f.git.annotation)
	}
}

func TestProvePublicationRequiresSourceTagAndPlatformDigests(t *testing.T) {
	f := newPubFixture(t)
	f.git.tagFound = false
	if _, err := f.prove(); err == nil || !strings.Contains(err.Error(), "source tag missing") {
		t.Fatalf("err = %v", err)
	}
	f = newPubFixture(t)
	f.git.sourceErr = fmt.Errorf("not an ancestor")
	if _, err := f.prove(); err == nil {
		t.Fatal("unverified source accepted")
	}
	f = newPubFixture(t)
	f.git.tagErr = fmt.Errorf("existing remote tag conflicts with verified provenance")
	if _, err := f.prove(); err == nil {
		t.Fatal("conflicting source tag accepted")
	}
	// The receipt names another platform digest than the published index.
	f = newPubFixture(t)
	f.receipt["platforms"].([]any)[0].(map[string]any)["digest"] = "sha256:" + strings.Repeat("0", 64)
	if _, err := f.prove(); err == nil || !strings.Contains(err.Error(), "platform digests differ") {
		t.Fatalf("err = %v", err)
	}
}

func TestProvePublicationChecksRegistryIndexAliasesAndLabels(t *testing.T) {
	for name, mutate := range map[string]func(*pubFixture){
		"version tag points elsewhere": func(f *pubFixture) { f.docs[pubTag] = []byte(`{"manifests":[]}`) },
		"stable alias differs":         func(f *pubFixture) { f.docs["stable"] = []byte(`{"manifests":[]}`) },
		"latest alias missing":         func(f *pubFixture) { delete(f.docs, "latest") },
		"index by digest differs":      func(f *pubFixture) { f.docs[f.index] = []byte(`{"manifests":[]}`) },
		"one architecture":             func(f *pubFixture) { f.docs[pubTag] = nil; f.setIndexOneArch() },
		"label mismatch": func(f *pubFixture) {
			f.labels["org.opencontainers.image.revision"] = strings.Repeat("b", 40)
			f.setRow(f.row.Name)
			f.setIndex(f.row.ManifestDigest, f.arm)
			f.receipt["digest"] = f.index
			f.receipt["reference"] = "quay.io/bundledock/toolbox@" + f.index
			f.setPlatforms()
		},
	} {
		f := newPubFixture(t)
		mutate(f)
		if _, err := f.prove(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func (f *pubFixture) setIndexOneArch() {
	index := map[string]any{"manifests": []any{map[string]any{"digest": f.row.ManifestDigest, "platform": map[string]any{"os": "linux", "architecture": "amd64"}}}}
	raw, _ := json.Marshal(index)
	f.index = jsonDoc(f.t, f.docs, index)
	f.docs[pubTag] = raw
	f.receipt["digest"] = f.index
	f.receipt["reference"] = "quay.io/bundledock/toolbox@" + f.index
}

func TestProvePublicationValidatesTheReceipt(t *testing.T) {
	for name, mutate := range map[string]func(*pubFixture){
		"foreign image":      func(f *pubFixture) { f.receipt["image"] = "quay.io/bundledock/bun" },
		"wrong commit":       func(f *pubFixture) { f.receipt["commit"] = strings.Repeat("b", 40) },
		"wrong source":       func(f *pubFixture) { f.receipt["source"] = "https://example.com/x" },
		"wrong run":          func(f *pubFixture) { f.receipt["run_id"] = "999" },
		"bad tag":            func(f *pubFixture) { f.receipt["tag"] = "stable" },
		"wrong attempt":      func(f *pubFixture) { f.env.RunAttempt = "2" },
		"bad created":        func(f *pubFixture) { f.receipt["created"] = "yesterday" },
		"bad lock":           func(f *pubFixture) { f.receipt["lock_sha256"] = "abc" },
		"bad digest":         func(f *pubFixture) { f.receipt["digest"] = "sha256:xyz" },
		"reference unpinned": func(f *pubFixture) { f.receipt["reference"] = "quay.io/bundledock/toolbox:" + pubTag },
		"one platform":       func(f *pubFixture) { f.receipt["platforms"] = f.receipt["platforms"].([]any)[:1] },
		"duplicate platform": func(f *pubFixture) {
			p := f.receipt["platforms"].([]any)
			p[1].(map[string]any)["architecture"] = "amd64"
		},
		"not smoked": func(f *pubFixture) { f.receipt["platforms"].([]any)[0].(map[string]any)["smoked"] = false },
		"platform differs": func(f *pubFixture) {
			f.receipt["platforms"].([]any)[1].(map[string]any)["lock_sha256"] = strings.Repeat("3", 64)
		},
		"platform image_id": func(f *pubFixture) { f.receipt["platforms"].([]any)[1].(map[string]any)["image_id"] = "none" },
		"other bundle":      func(f *pubFixture) { f.receipt["bundle"] = "bun" },
		"bad sha env":       func(f *pubFixture) { f.env.SHA = "main" },
		"bad run env":       func(f *pubFixture) { f.env.RunID = "0" },
	} {
		f := newPubFixture(t)
		mutate(f)
		if _, err := f.prove(); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if len(f.git.verified) != 0 {
			t.Errorf("%s: source checked before the receipt was valid", name)
		}
	}
	f := newPubFixture(t)
	if _, err := f.pruner.ProvePublication(context.Background(), []byte(`[]`), "toolbox", f.env); err == nil {
		t.Error("non-object receipt accepted")
	}
	if _, err := f.pruner.ProvePublication(context.Background(), []byte(`{`), "toolbox", f.env); err == nil {
		t.Error("malformed receipt accepted")
	}
	if _, err := f.pruner.ProvePublication(context.Background(), []byte(`{}`), "nope", f.env); err == nil {
		t.Error("unknown bundle accepted")
	}
}

func TestProvePublicationDailyAndSemanticTagsNeedTheExplicitAttempt(t *testing.T) {
	f := newPubFixture(t)
	f.labels["org.opencontainers.image.version"] = "2026.10.01-b4"
	f.setRow("staging-2026.10.01-b4-amd64")
	armConfig := jsonDoc(t, f.docs, map[string]any{"os": "linux", "architecture": "arm64", "config": map[string]any{"Labels": f.labels}})
	f.arm = jsonDoc(t, f.docs, map[string]any{"config": map[string]any{"digest": armConfig}})
	f.setIndex(f.row.ManifestDigest, f.arm)
	f.docs["2026.10.01-b4"] = f.docs["stable"]
	f.receipt["tag"], f.receipt["digest"], f.receipt["reference"] = "2026.10.01-b4", f.index, "quay.io/bundledock/toolbox@"+f.index
	f.setPlatforms()
	if _, err := f.prove(); err == nil {
		t.Fatal("daily tag without run_attempt accepted")
	}
	f.receipt["run_attempt"] = "1"
	for _, p := range f.receipt["platforms"].([]any) {
		p.(map[string]any)["run_attempt"] = "1"
	}
	p, err := f.prove()
	if err != nil || p.Attempt != 1 {
		t.Fatalf("p = %+v err = %v", p, err)
	}
	f.receipt["run_attempt"] = 1 // a number is not the attempt string
	if _, err := f.prove(); err == nil {
		t.Fatal("numeric run_attempt accepted")
	}
}

// --- assess, plan and apply in publication mode ------------------------------

func TestPublicationStagingNeedsTheExactReceiptPlatformDigest(t *testing.T) {
	f := newPubFixture(t)
	f.pruner.Publication = f.publication()
	// The publication run itself may still be running while it cleans up.
	f.gh.run.Status = "in_progress"
	row := f.assess()
	if !row.Eligible || row.Reason != "Verified publication staging tag" {
		t.Fatalf("row = %+v", row)
	}
	f.pruner.Publication.Platforms["amd64"] = "sha256:" + strings.Repeat("f", 64)
	if f.assess().Eligible {
		t.Fatal("tag with another digest than the receipt is eligible")
	}
}

func TestPublicationStagingChecksIdentityAttemptAndTime(t *testing.T) {
	for name, mutate := range map[string]func(*pubFixture){
		"bundle":     func(f *pubFixture) { f.pruner.Publication.Bundle = "bun" },
		"tag":        func(f *pubFixture) { f.pruner.Publication.Tag = "2026.10.01-b5.1" },
		"commit":     func(f *pubFixture) { f.pruner.Publication.Commit = strings.Repeat("b", 40) },
		"run":        func(f *pubFixture) { f.pruner.Publication.RunID = "124" },
		"attempt":    func(f *pubFixture) { f.pruner.Publication.Attempt = 2 },
		"rerun":      func(f *pubFixture) { f.gh.run.RunAttempt = 2 },
		"future tag": func(f *pubFixture) { f.row.LastModified = "Thu, 15 Oct 2026 00:00:00 -0000" },
		"other run":  func(f *pubFixture) { f.gh.run.Path = ".github/workflows/ci.yml" },
	} {
		f := newPubFixture(t)
		f.pruner.Publication = f.publication()
		mutate(f)
		if f.assess().Eligible {
			t.Errorf("%s: eligible", name)
		}
	}
}

func TestPublicationModeConflictsWithFailedRun(t *testing.T) {
	f := newPubFixture(t)
	f.pruner.Publication = f.publication()
	f.pruner.FailedRun = &FailedRun{RunID: "123", Attempt: 1}
	if row := f.assess(); row.Eligible || row.Reason != "Conflicting cleanup modes" {
		t.Fatalf("row = %+v", row)
	}
	if _, err := f.pruner.Plan(context.Background()); err == nil {
		t.Fatal("plan with both modes accepted")
	}
}

func TestPublicationPlanListsOnlyTheReceiptStagingTags(t *testing.T) {
	f := newPubFixture(t)
	f.pruner.Publication = f.publication()
	f.store.rows = append(f.store.rows,
		Tag{Name: "staging-2026.10.01-b5.1-amd64", ManifestDigest: f.row.ManifestDigest}, // another version
		Tag{Name: "staging-" + pubTag + "-arm64", ManifestDigest: f.arm, LastModified: f.row.LastModified},
		Tag{Name: "stable"}, Tag{Name: pubTag})
	got, err := f.pruner.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModePublication || got.Target != nil || len(got.Rows) != 2 || got.Rows[0].Name != "staging-"+pubTag+"-amd64" || got.Rows[1].Name != "staging-"+pubTag+"-arm64" {
		t.Fatalf("plan = %+v", got)
	}
	if !got.Rows[0].Eligible || len(f.store.deleted) != 0 {
		t.Fatalf("plan = %+v deleted = %v", got, f.store.deleted)
	}
}

func TestPublicationApplyDeletesAfterRecheckAndRejectsForeignRows(t *testing.T) {
	f := newPubFixture(t)
	f.pruner.Publication = f.publication()
	approved := Plan{Schema: 1, Mode: ModePublication, Rows: []PlanRow{f.assess()}}
	actions, err := f.pruner.Apply(context.Background(), approved, nil)
	if err != nil || len(actions) != 1 || actions[0].Action != "deleted" || f.store.deleted[0] != "toolbox/"+f.row.Name {
		t.Fatalf("actions = %v err = %v deleted = %v", actions, err, f.store.deleted)
	}
	// A plan row that the receipt does not name is rejected before any deletion.
	f = newPubFixture(t)
	f.pruner.Publication = f.publication()
	good := f.assess()
	other := good
	other.Name = "staging-2026.10.01-b5.1-amd64"
	for name, rows := range map[string][]PlanRow{"other version": {good, other}} {
		if _, err := f.pruner.Apply(context.Background(), Plan{Schema: 1, Mode: ModePublication, Rows: rows}, nil); err == nil || len(f.store.deleted) != 0 {
			t.Errorf("%s: err = %v deleted = %v", name, err, f.store.deleted)
		}
	}
	// The plan mode and target must match the request.
	if _, err := f.pruner.Apply(context.Background(), Plan{Schema: 1, Mode: ModeFallback, Rows: []PlanRow{good}}, nil); err == nil {
		t.Error("fallback plan accepted for a publication request")
	}
	if _, err := f.pruner.Apply(context.Background(), Plan{Schema: 1, Mode: ModePublication, Target: &FailedRun{RunID: "123", Attempt: 1}, Rows: []PlanRow{good}}, nil); err == nil {
		t.Error("publication plan with a target accepted")
	}
	fallback := newPruneFixture(t)
	if _, err := fallback.pruner.Apply(context.Background(), Plan{Schema: 1, Mode: ModePublication, Rows: []PlanRow{good}}, nil); err == nil || len(fallback.store.deleted) != 0 {
		t.Errorf("publication plan accepted by a fallback request: %v", err)
	}
	// A receipt platform that changed since the dry run stops the deletion.
	f.pruner.Publication.Platforms["amd64"] = "sha256:" + strings.Repeat("e", 64)
	if _, err := f.pruner.Apply(context.Background(), Plan{Schema: 1, Mode: ModePublication, Rows: []PlanRow{good}}, nil); err == nil || len(f.store.deleted) != 0 {
		t.Errorf("recheck against another receipt: err = %v deleted = %v", err, f.store.deleted)
	}
}

func TestRegistryManifestRefReadsOnlyTagsAliasesAndDigests(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v2/auth" {
			w.Write([]byte(`{"token":"tok"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("manifest read without registry token")
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	reg, err := New(srv.Client(), srv.URL).Registry(context.Background(), "toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{pubTag, "2026.10.01-b4", "26.12.0-b1", "2026.10.01-5-6", "stable", "latest", "lts", "current"} {
		if _, err := reg.ManifestRef(context.Background(), ref); err != nil {
			t.Errorf("%s: %v", ref, err)
		}
	}
	before := len(paths)
	for _, ref := range []string{"", "../stable", "staging-" + pubTag + "-amd64", "main", pubTag + "/x", "stable\n"} {
		if _, err := reg.ManifestRef(context.Background(), ref); err == nil {
			t.Errorf("%q accepted", ref)
		}
	}
	if len(paths) != before {
		t.Fatalf("a rejected reference reached the network: %v", paths[before:])
	}
}
