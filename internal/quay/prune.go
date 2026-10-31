package quay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// This file is the plan logic of the former Python cleanup script
// (fallback, failed-run and, in publication.go, publication modes). It only
// ever selects staging tags; version, alias and source tags are never touched.

const (
	// SourceURL is the only accepted source repository of a staging image.
	SourceURL = "https://github.com/wstein/bundledock"
	// GitHubRepository owns the publication workflows.
	GitHubRepository = "wstein/bundledock"

	ModeFallback  = "fallback"
	ModeFailedRun = "failed-run"
)

var (
	dailyPattern     = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2})-b([1-9]\d*)$`)
	releasePattern   = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2})-b([1-9]\d*)\.([1-9]\d*)$`)
	singletonPattern = regexp.MustCompile(`^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)-b[1-9]\d*$`)
	legacyPattern    = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2})-([1-9]\d*)-([1-9]\d*)$`)
	positivePattern  = regexp.MustCompile(`^[1-9][0-9]*$`)
	commitPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// reasonError is a verification failure that is safe to show in a plan.
// Any other error (transport, malformed evidence) is reported generically.
type reasonError string

func (e reasonError) Error() string { return string(e) }

func check(ok bool, reason string) error {
	if ok {
		return nil
	}
	return reasonError(reason)
}

func first(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Run is the GitHub Actions evidence for a workflow run.
type Run struct {
	ID         int64  `json:"id"`
	RunNumber  int    `json:"run_number"`
	RunAttempt int    `json:"run_attempt"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	// ConclusionNull is true when GitHub reports no conclusion yet (null or absent).
	ConclusionNull bool   `json:"-"`
	UpdatedAt      string `json:"updated_at"`
	Event          string `json:"event"`
	HeadBranch     string `json:"head_branch"`
	HeadSHA        string `json:"head_sha"`
	WorkflowID     int64  `json:"workflow_id"`
	Path           string `json:"path"`
	Repository     struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadRepository struct {
		FullName string `json:"full_name"`
	} `json:"head_repository"`
}

// UnmarshalJSON records whether the conclusion is null.
func (r *Run) UnmarshalJSON(data []byte) error {
	type plain Run
	aux := struct {
		*plain
		Conclusion *string `json:"conclusion"`
	}{plain: (*plain)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.ConclusionNull = aux.Conclusion == nil
	r.Conclusion = ""
	if aux.Conclusion != nil {
		r.Conclusion = *aux.Conclusion
	}
	return nil
}

// Workflow is the GitHub evidence for the publication workflow.
type Workflow struct {
	ID    int64  `json:"id"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// Job is one job of a workflow run attempt.
type Job struct {
	ID         int64  `json:"id"`
	RunID      int64  `json:"run_id"`
	RunAttempt int    `json:"run_attempt"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// GitHub supplies workflow evidence. fresh bypasses any cache.
type GitHub interface {
	Run(ctx context.Context, runID string, fresh bool) (Run, error)
	Attempt(ctx context.Context, runID string, attempt int, fresh bool) (Run, error)
	Workflow(ctx context.Context, fresh bool) (Workflow, error)
	Jobs(ctx context.Context, runID string, attempt int, fresh bool) ([]Job, error)
}

// Evidence reads registry content by digest (verified by the implementation).
type Evidence interface {
	Manifest(ctx context.Context, digest string) ([]byte, error)
	Blob(ctx context.Context, digest string) ([]byte, error)
	// ManifestRef reads a manifest by version tag or alias name; the caller
	// verifies its digest.
	ManifestRef(ctx context.Context, ref string) ([]byte, error)
}

// TagStore lists and removes tags.
type TagStore interface {
	// Tags lists the tags of a repository, or only name when it is not empty.
	Tags(ctx context.Context, repository, name string) ([]Tag, error)
	// Delete removes exactly one staging tag.
	Delete(ctx context.Context, repository, name string) error
}

// FailedRun selects the exact failed publication attempt to clean up.
type FailedRun struct {
	RunID   string `json:"run_id"`
	Attempt int    `json:"attempt"`
}

// PlanRow is the assessment of one staging tag.
type PlanRow struct {
	Repository     string `json:"repository"`
	Name           string `json:"name"`
	ManifestDigest string `json:"manifest_digest"`
	LastModified   string `json:"last_modified"`
	Eligible       bool   `json:"eligible"`
	Reason         string `json:"reason,omitempty"`
	Bundle         string `json:"bundle,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	Commit         string `json:"commit,omitempty"`
	Version        string `json:"version,omitempty"`
	Architecture   string `json:"architecture,omitempty"`
}

// Plan is the saved dry-run result that --apply consumes.
type Plan struct {
	Schema      int        `json:"schema"`
	Mode        string     `json:"mode"`
	GeneratedAt string     `json:"generated_at"`
	Target      *FailedRun `json:"target,omitempty"`
	Rows        []PlanRow  `json:"rows"`
}

// Action is the result for one approved tag.
type Action struct {
	Repository string `json:"repository"`
	Name       string `json:"name"`
	Action     string `json:"action"`
}

// Pruner plans and applies staging-tag cleanup.
type Pruner struct {
	Store TagStore
	// Registry opens evidence for a repository, on first use only.
	Registry func(ctx context.Context, repository string) (Evidence, error)
	GitHub   GitHub
	Now      func() time.Time
	// Repos is the selected scope.
	Repos     []string
	FailedRun *FailedRun
	// Publication selects the publication mode; ProvePublication sets it.
	Publication *Publication
	// Git verifies the source of a publication.
	Git Git

	evidence map[string]Evidence
}

// Repositories lists the distinct Quay repositories of all bundles, sorted.
func Repositories() []string {
	seen := map[string]bool{}
	var result []string
	for _, b := range Bundles() {
		if !seen[b.Repository] {
			seen[b.Repository] = true
			result = append(result, b.Repository)
		}
	}
	for i := range result {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func knownRepository(repository string) bool {
	for _, r := range Repositories() {
		if r == repository {
			return true
		}
	}
	return false
}

// Identity validates a staging tag name and returns its version and architecture.
func Identity(repository, name string) (version, arch string, err error) {
	if !knownRepository(repository) {
		return "", "", reasonError("Unknown repository")
	}
	match := stagingPattern.FindStringSubmatch(name)
	if match == nil {
		return "", "", reasonError("Not an architecture staging tag")
	}
	version, arch = match[1], match[2]
	if !(dailyPattern.MatchString(version) || releasePattern.MatchString(version) || singletonPattern.MatchString(version) || legacyPattern.MatchString(version)) {
		return "", "", reasonError("Unknown staging version")
	}
	return version, arch, nil
}

var failedConclusions = map[string]bool{"failure": true, "cancelled": true, "timed_out": true}

func sameAttempt(a, b Run) bool {
	return a.ID == b.ID && a.RunAttempt == b.RunAttempt && a.Status == b.Status && a.Conclusion == b.Conclusion && a.ConclusionNull == b.ConclusionNull &&
		a.Path == b.Path && a.Event == b.Event && a.HeadBranch == b.HeadBranch && a.HeadSHA == b.HeadSHA && a.WorkflowID == b.WorkflowID
}

// ProveFailedRun checks that the selected run is a completed failed
// publication attempt of the official workflow, with no active rerun.
func ProveFailedRun(ctx context.Context, gh GitHub, runID string, attempt int, fresh bool, run *Run) (Run, error) {
	if err := check(positivePattern.MatchString(runID) && attempt > 0, "Invalid failed run selection"); err != nil {
		return Run{}, err
	}
	if run == nil {
		got, err := gh.Run(ctx, runID, fresh)
		if err != nil {
			return Run{}, err
		}
		run = &got
	}
	r := *run
	if err := first(
		check(strconv.FormatInt(r.ID, 10) == runID && r.RunAttempt == attempt, "Failed run was rerun or its attempt changed"),
		check(r.Status == "completed" && !r.ConclusionNull && failedConclusions[r.Conclusion], "Selected run is not a completed failed or cancelled publication"),
		check(r.Repository.FullName == GitHubRepository && r.HeadRepository.FullName == GitHubRepository, "Foreign run repository"),
		check(r.HeadBranch == "main" && (r.Event == "workflow_dispatch" || r.Event == "workflow_run"), "Not an official main publication event"),
		check(r.Path == ".github/workflows/publish-bundles.yml" && commitPattern.MatchString(r.HeadSHA), "Not the official publication workflow"),
	); err != nil {
		return Run{}, err
	}
	workflow, err := gh.Workflow(ctx, fresh)
	if err != nil {
		return Run{}, err
	}
	if err := check(workflow.ID == r.WorkflowID && workflow.Path == r.Path && workflow.State == "active", "Publication workflow identity unavailable or changed"); err != nil {
		return Run{}, err
	}
	exact, err := gh.Attempt(ctx, runID, attempt, fresh)
	if err != nil {
		return Run{}, err
	}
	if err := check(sameAttempt(exact, r), "Exact attempt evidence differs from latest run"); err != nil {
		return Run{}, err
	}
	jobs, err := gh.Jobs(ctx, runID, attempt, fresh)
	if err != nil {
		return Run{}, err
	}
	if err := check(len(jobs) > 0 && len(jobs) <= 2000, "Complete job evidence unavailable"); err != nil {
		return Run{}, err
	}
	seen := map[int64]bool{}
	for _, job := range jobs {
		if err := first(
			check(!seen[job.ID] && strconv.FormatInt(job.RunID, 10) == runID && job.RunAttempt == attempt, "Job belongs to another run or attempt"),
			check(job.Status == "completed" && job.Conclusion != "", "Publication job still active or unverifiable"),
		); err != nil {
			return Run{}, err
		}
		seen[job.ID] = true
	}
	// Resample after the attempt and jobs reads; a queued or active rerun invalidates the proof.
	latest, err := gh.Run(ctx, runID, true)
	if err != nil {
		return Run{}, err
	}
	if err := check(sameAttempt(latest, r), "Publication changed while checking completed jobs"); err != nil {
		return Run{}, err
	}
	return r, nil
}

// stagingRecord is the identity that labels and the staging name must agree on.
type stagingRecord struct {
	bundle, image, tag, digest, source, commit, runID string
	runAttempt                                        string // label value, "" when absent
	hasAttempt                                        bool
	legacy                                            bool
	attempt                                           int
	runNumber                                         int // 0 when none
}

func validDate(s string) bool {
	_, err := time.Parse("2006.01.02", s)
	return err == nil
}

func validateRecord(r *stagingRecord) error {
	b, err := FindBundle(r.bundle)
	if err != nil || r.image != "quay.io/"+Namespace+"/"+b.Repository {
		return reasonError("unexpected official bundle repository")
	}
	if r.source != SourceURL || !commitPattern.MatchString(r.commit) {
		return reasonError("unexpected source repository or revision")
	}
	if !digestPattern.MatchString(r.digest) || !positivePattern.MatchString(r.runID) {
		return reasonError("invalid index digest or workflow run")
	}
	switch {
	case dailyPattern.MatchString(r.tag):
		if !validDate(r.tag[:10]) {
			return reasonError("expected an immutable version tag")
		}
	case singletonPattern.MatchString(r.tag):
	default:
		match := releasePattern.FindStringSubmatch(r.tag)
		if match == nil {
			match = legacyPattern.FindStringSubmatch(r.tag)
			r.legacy = match != nil
			if match != nil && match[2] != r.runID {
				return reasonError("legacy run ID mismatch")
			}
		}
		if match == nil || !validDate(match[1]) {
			return reasonError("expected an immutable version tag")
		}
		r.attempt, _ = strconv.Atoi(match[3])
		if !r.legacy {
			r.runNumber, _ = strconv.Atoi(match[2])
		}
	}
	if dailyPattern.MatchString(r.tag) || singletonPattern.MatchString(r.tag) {
		dated := r.bundle == "toolbox" || r.bundle == "nixbox"
		if (dated && !dailyPattern.MatchString(r.tag)) || !r.hasAttempt || !positivePattern.MatchString(r.runAttempt) {
			return reasonError("singleton version requires explicit workflow attempt")
		}
		r.attempt, _ = strconv.Atoi(r.runAttempt)
	}
	return nil
}

func verifyLabels(r *stagingRecord, labels map[string]string) error {
	identity, has := labels["io.bundledock.bundle"]
	if (has && identity != r.bundle) || (!has && (r.bundle == "node-current" || (r.bundle == "node" && singletonPattern.MatchString(r.tag)))) {
		return reasonError("OCI source bundle mismatch")
	}
	if dailyPattern.MatchString(r.tag) || singletonPattern.MatchString(r.tag) {
		if labels["io.bundledock.github.run-attempt"] != strconv.Itoa(r.attempt) {
			return reasonError("OCI workflow attempt mismatch")
		}
	}
	for key, want := range map[string]string{"source": r.source, "revision": r.commit, "version": r.tag} {
		if got, ok := labels["org.opencontainers.image."+key]; !ok || got != want {
			return reasonError("OCI " + key + " mismatch")
		}
	}
	runID, ok := labels["io.bundledock.github.run-id"]
	if runID != r.runID && !(r.legacy && !ok) {
		return reasonError("OCI workflow run mismatch")
	}
	return nil
}

func (p *Pruner) evidenceFor(ctx context.Context, repository string) (Evidence, error) {
	if e, ok := p.evidence[repository]; ok {
		return e, nil
	}
	e, err := p.Registry(ctx, repository)
	if err != nil {
		return nil, err
	}
	if p.evidence == nil {
		p.evidence = map[string]Evidence{}
	}
	p.evidence[repository] = e
	return e, nil
}

// Assess verifies one staging tag. Anything unverifiable makes it ineligible.
func (p *Pruner) Assess(ctx context.Context, repository string, row Tag, fresh bool) PlanRow {
	result := PlanRow{Repository: repository, Name: row.Name, ManifestDigest: row.ManifestDigest, LastModified: row.LastModified}
	reason, err := p.assess(ctx, repository, row, fresh, &result)
	if err == nil {
		result.Eligible, result.Reason = true, reason
		return result
	}
	var known reasonError
	if errors.As(err, &known) {
		result.Reason = string(known)
	} else {
		// Never include response bodies, tokens or signed URLs in audit output.
		result.Reason = "Required provenance evidence unavailable"
	}
	return result
}

func (p *Pruner) assess(ctx context.Context, repository string, row Tag, fresh bool, result *PlanRow) (string, error) {
	version, arch, err := Identity(repository, row.Name)
	if err != nil {
		return "", err
	}
	if err := check(!row.IsManifestList && !row.Immutable, "Index or immutable tag retained"); err != nil {
		return "", err
	}
	evidence, err := p.evidenceFor(ctx, repository)
	if err != nil {
		return "", err
	}
	manifestRaw, err := evidence.Manifest(ctx, row.ManifestDigest)
	if err != nil {
		return "", err
	}
	var manifest struct {
		Manifests []any `json:"manifests"`
		Config    struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := unmarshal(manifestRaw, &manifest); err != nil {
		return "", err
	}
	if err := check(manifest.Manifests == nil, "Index retained"); err != nil {
		return "", err
	}
	configRaw, err := evidence.Blob(ctx, manifest.Config.Digest)
	if err != nil {
		return "", err
	}
	var config struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := unmarshal(configRaw, &config); err != nil {
		return "", err
	}
	if err := check(config.OS == "linux" && config.Architecture == arch, "Platform differs from staging name"); err != nil {
		return "", err
	}
	labels := config.Config.Labels
	if labels == nil {
		return "", errors.New("no labels")
	}
	bundle, hasBundle := labels["io.bundledock.bundle"]
	// Historical single-channel images may predate the explicit bundle label.
	if !hasBundle && repository != "node" {
		bundle = repository
	}
	b, err := FindBundle(bundle)
	if err := check(err == nil && b.Repository == repository, "Source bundle unavailable or mismatched"); err != nil {
		return "", err
	}
	runID, ok := labels["io.bundledock.github.run-id"]
	source, ok2 := labels["org.opencontainers.image.source"]
	commit, ok3 := labels["org.opencontainers.image.revision"]
	if !ok || !ok2 || !ok3 {
		return "", errors.New("required labels missing")
	}
	attemptLabel, hasAttempt := labels["io.bundledock.github.run-attempt"]
	record := &stagingRecord{bundle: bundle, image: "quay.io/" + Namespace + "/" + repository, tag: version, digest: row.ManifestDigest,
		source: source, commit: commit, runID: runID, runAttempt: attemptLabel, hasAttempt: hasAttempt}
	if err := validateRecord(record); err != nil {
		return "", err
	}
	if err := verifyLabels(record, labels); err != nil {
		return "", err
	}
	if err := check(labels["org.opencontainers.image.version"] == version, "Version differs from staging name"); err != nil {
		return "", err
	}
	attempt := record.attempt
	if hasAttempt {
		if err := check(attemptLabel == strconv.Itoa(attempt), "Attempt mismatch"); err != nil {
			return "", err
		}
	}
	if p.FailedRun != nil {
		if err := check(p.Publication == nil, "Conflicting cleanup modes"); err != nil {
			return "", err
		}
		if err := check(runID == p.FailedRun.RunID && attempt == p.FailedRun.Attempt, "Tag belongs to another run or attempt"); err != nil {
			return "", err
		}
	}
	run, err := p.GitHub.Run(ctx, runID, fresh)
	if err != nil {
		return "", err
	}
	if err := first(
		check(strconv.FormatInt(run.ID, 10) == runID && run.Repository.FullName == GitHubRepository && run.HeadBranch == "main", "Wrong run provenance"),
		check(run.Path == ".github/workflows/publish-bundles.yml" || run.Path == ".github/workflows/bundledock-"+bundle+".yml", "Not a publication workflow"),
		check(run.Event == "workflow_dispatch" || run.Event == "workflow_run", "Unexpected run event"),
	); err != nil {
		return "", err
	}
	if run.Event == "workflow_dispatch" {
		if err := check(run.HeadSHA == record.commit, "Run source mismatch"); err != nil {
			return "", err
		}
	}
	if err := check(run.RunAttempt >= attempt, "Run attempt mismatch"); err != nil {
		return "", err
	}
	if record.runNumber != 0 {
		if err := check(run.RunNumber == record.runNumber, "Run number mismatch"); err != nil {
			return "", err
		}
	}
	result.Bundle, result.RunID, result.Attempt, result.Commit, result.Version, result.Architecture = bundle, runID, attempt, record.commit, version, arch
	modified, err := row.Modified()
	if err != nil {
		return "", err
	}
	now := p.Now()
	if pub := p.Publication; pub != nil {
		if err := first(
			check(bundle == pub.Bundle && version == pub.Tag && record.commit == pub.Commit && runID == pub.RunID && attempt == pub.Attempt, "Receipt identity mismatch"),
			check(pub.Platforms[arch] == row.ManifestDigest && run.RunAttempt == attempt, "Receipt platform or current attempt mismatch"),
			check(!modified.After(now), "Future staging timestamp"),
		); err != nil {
			return "", err
		}
		return "Verified publication staging tag", nil
	}
	updated, err := time.Parse(time.RFC3339, run.UpdatedAt)
	if err != nil {
		return "", err
	}
	if p.FailedRun != nil {
		if _, err := ProveFailedRun(ctx, p.GitHub, p.FailedRun.RunID, p.FailedRun.Attempt, fresh, &run); err != nil {
			return "", err
		}
		if err := first(check(!modified.After(now), "Future staging timestamp"), check(!updated.After(now), "Future or invalid completed-run timestamp")); err != nil {
			return "", err
		}
		return "Verified exact completed failed publication attempt", nil
	}
	cutoff := now.AddDate(0, 0, -7)
	if err := first(
		check(!modified.After(cutoff), "Staging tag younger than seven days"),
		check(run.Status == "completed" && !run.ConclusionNull, "Publication run still active"),
		check(!updated.After(cutoff), "Run or rerun younger than seven days"),
	); err != nil {
		return "", err
	}
	return "Verified completed run and tag older than seven days", nil
}

// Plan lists and assesses every staging tag of the selected repositories.
func (p *Pruner) Plan(ctx context.Context) (Plan, error) {
	now := p.Now()
	if err := check(p.Publication == nil || p.FailedRun == nil, "Conflicting cleanup modes"); err != nil {
		return Plan{}, err
	}
	plan := Plan{Schema: 1, Mode: p.mode(), GeneratedAt: now.Format(time.RFC3339Nano), Target: p.FailedRun, Rows: []PlanRow{}}
	if p.FailedRun != nil {
		if _, err := ProveFailedRun(ctx, p.GitHub, p.FailedRun.RunID, p.FailedRun.Attempt, true, nil); err != nil {
			return Plan{}, err
		}
	}
	for _, repository := range p.Repos {
		tags, err := p.Store.Tags(ctx, repository, "")
		if err != nil {
			return Plan{}, err
		}
		for _, tag := range tags {
			if len(tag.Name) >= 8 && tag.Name[:8] == "staging-" && (p.Publication == nil || p.Publication.names(tag.Name)) {
				plan.Rows = append(plan.Rows, p.Assess(ctx, repository, tag, false))
			}
		}
	}
	sortRows(plan.Rows)
	return plan, nil
}

func (p *Pruner) mode() string {
	if p.Publication != nil {
		return ModePublication
	}
	if p.FailedRun != nil {
		return ModeFailedRun
	}
	return ModeFallback
}

func (p *Pruner) inScope(repository string) bool {
	for _, r := range p.Repos {
		if r == repository {
			return true
		}
	}
	return false
}

// Apply deletes the eligible tags of a saved plan, each after a fresh recheck.
// Every approved row is validated before the first deletion. report is
// called after each action; its error stops the run.
func (p *Pruner) Apply(ctx context.Context, plan Plan, report func([]Action) error) ([]Action, error) {
	if err := check(plan.Schema == 1 && (plan.Mode == ModeFallback || plan.Mode == ModeFailedRun || plan.Mode == ModePublication), "Invalid cleanup plan"); err != nil {
		return nil, err
	}
	if err := check(plan.Mode == p.mode(), "Plan mode differs from request"); err != nil {
		return nil, err
	}
	if err := check((plan.Target == nil) == (p.FailedRun == nil) && (plan.Target == nil || *plan.Target == *p.FailedRun), "Plan target differs from request"); err != nil {
		return nil, err
	}
	if plan.Mode == ModeFailedRun {
		if err := check(positivePattern.MatchString(plan.Target.RunID) && plan.Target.Attempt > 0, "Invalid failed-run plan target"); err != nil {
			return nil, err
		}
	}
	var approved []PlanRow
	seen := map[[2]string]bool{}
	for _, row := range plan.Rows {
		if !row.Eligible {
			continue
		}
		if err := check(p.inScope(row.Repository), "Plan repository outside selected scope"); err != nil {
			return nil, err
		}
		if _, _, err := Identity(row.Repository, row.Name); err != nil {
			return nil, err
		}
		if plan.Mode == ModePublication {
			if err := check(p.Publication.names(row.Name), "Plan row outside the publication receipt"); err != nil {
				return nil, err
			}
		}
		if plan.Mode == ModeFailedRun {
			if err := check(row.RunID == plan.Target.RunID && row.Attempt == plan.Target.Attempt, "Plan row outside exact failed attempt"); err != nil {
				return nil, err
			}
		}
		key := [2]string{row.Repository, row.Name}
		if err := first(check(digestPattern.MatchString(row.ManifestDigest), "Invalid approved digest"), check(!seen[key], "Duplicate approved tag")); err != nil {
			return nil, err
		}
		seen[key] = true
		approved = append(approved, row)
	}
	actions := []Action{}
	note := func(row PlanRow, action string) error {
		actions = append(actions, Action{Repository: row.Repository, Name: row.Name, Action: action})
		if report != nil {
			return report(actions)
		}
		return nil
	}
	for _, row := range approved {
		current, err := p.Store.Tags(ctx, row.Repository, row.Name)
		if err != nil {
			return actions, err
		}
		if len(current) == 0 {
			if err := note(row, "already absent"); err != nil {
				return actions, err
			}
			continue
		}
		if err := check(len(current) == 1 && current[0].Name == row.Name && current[0].ManifestDigest == row.ManifestDigest && current[0].LastModified == row.LastModified, "Staging tag changed since dry run"); err != nil {
			return actions, err
		}
		verified := p.Assess(ctx, row.Repository, current[0], true)
		if err := check(verified.Eligible && verified.Repository == row.Repository && verified.Name == row.Name && verified.ManifestDigest == row.ManifestDigest &&
			verified.LastModified == row.LastModified && verified.Bundle == row.Bundle && verified.RunID == row.RunID && verified.Attempt == row.Attempt, "Staging tag no longer eligible"); err != nil {
			return actions, err
		}
		if err := p.Store.Delete(ctx, row.Repository, row.Name); err != nil {
			return actions, err
		}
		if err := note(row, "deleted"); err != nil {
			return actions, err
		}
	}
	return actions, nil
}

func sortRows(rows []PlanRow) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && (rows[j].Repository < rows[j-1].Repository || (rows[j].Repository == rows[j-1].Repository && rows[j].Name < rows[j-1].Name)); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func unmarshal(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("malformed registry document")
	}
	return nil
}
