package quay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This file is the publication mode of prune (ported from the former Python
// cleanup script): a staging
// tag is removed because a verified publication receipt names exactly its
// platform digest, not because it is old.

// ModePublication removes the staging tags of one verified publication.
const ModePublication = "publication"

// Publication is a verified publication receipt.
type Publication struct {
	Bundle  string
	Tag     string
	Commit  string
	RunID   string
	Attempt int
	// Platforms maps an architecture to its platform manifest digest.
	Platforms map[string]string
}

// PublicationEnv is the expected provenance, from the publication run's
// environment (PUBLICATION_SHA, GITHUB_RUN_ID, GITHUB_RUN_ATTEMPT).
type PublicationEnv struct {
	SHA, RunID, RunAttempt string
}

var lockPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

func dated(bundle string) bool { return bundle == "toolbox" || bundle == "nixbox" }

// versionKind validates an immutable version tag (no legacy form) and returns
// the attempt a release tag carries; daily and semantic tags carry none.
func versionKind(tag string) (daily, semantic bool, attempt int, err error) {
	switch {
	case dailyPattern.MatchString(tag):
		if !validDate(tag[:10]) {
			return false, false, 0, reasonError("expected an immutable version tag")
		}
		return true, false, 0, nil
	case singletonPattern.MatchString(tag):
		return false, true, 0, nil
	}
	match := releasePattern.FindStringSubmatch(tag)
	if match == nil || !validDate(match[1]) {
		return false, false, 0, reasonError("expected an immutable version tag")
	}
	attempt, _ = strconv.Atoi(match[3])
	return false, false, attempt, nil
}

func text(m map[string]any, key string) (string, bool) {
	s, ok := m[key].(string)
	return s, ok
}

func digestOf(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }

func checkDigest(raw []byte, digest string) error {
	return check(digestPattern.MatchString(digest) && digestOf(raw) == digest, "registry content digest mismatch")
}

// validateReceipt is docs.validate of scripts/publication-docs.py. It returns
// the platform digests by architecture.
func validateReceipt(receipt map[string]any, b Bundle, env PublicationEnv) (map[string]string, error) {
	image := "quay.io/" + Namespace + "/" + b.Repository
	if err := check(commitPattern.MatchString(env.SHA) && positivePattern.MatchString(env.RunID) && positivePattern.MatchString(env.RunAttempt), "Invalid expected provenance"); err != nil {
		return nil, err
	}
	got := func(key string) string { s, _ := text(receipt, key); return s }
	if err := check(got("image") == image && got("commit") == env.SHA && got("source") == SourceURL && got("run_id") == env.RunID, "Wrong publication provenance"); err != nil {
		return nil, err
	}
	tag, ok := text(receipt, "tag")
	if err := check(ok, "Invalid publication version"); err != nil {
		return nil, err
	}
	daily, semantic, attempt, err := versionKind(tag)
	if err != nil {
		return nil, err
	}
	if v, present := receipt["bundle"]; present {
		if err := check(v == b.Name, "Wrong source bundle identity"); err != nil {
			return nil, err
		}
	}
	explicit := daily || semantic
	if explicit {
		if err := check(dated(b.Name) == daily && got("run_attempt") == env.RunAttempt, "Wrong publication attempt"); err != nil {
			return nil, err
		}
	} else if err := check(strconv.Itoa(attempt) == env.RunAttempt, "Wrong publication attempt"); err != nil {
		return nil, err
	}
	created := got("created")
	if _, err := time.Parse("2006-01-02T15:04:05Z", created); err != nil || !timestampPattern.MatchString(created) {
		return nil, reasonError("Invalid publication date")
	}
	if daily {
		if err := check(strings.ReplaceAll(tag[:10], ".", "-") == created[:10], "Publication date differs from tag"); err != nil {
			return nil, err
		}
	}
	if err := check(lockPattern.MatchString(got("lock_sha256")), "Invalid lock SHA256"); err != nil {
		return nil, err
	}
	digest := got("digest")
	if err := check(digestPattern.MatchString(digest), "Invalid index digest"); err != nil {
		return nil, err
	}
	if err := check(got("reference") == image+"@"+digest, "Reference must pin the index digest"); err != nil {
		return nil, err
	}
	platforms, _ := receipt["platforms"].([]any)
	if err := check(len(platforms) == 2, "Both platform receipts required"); err != nil {
		return nil, err
	}
	digests := map[string]string{}
	for _, item := range platforms {
		if p, ok := item.(map[string]any); ok {
			arch, _ := text(p, "architecture")
			digests[arch], _ = text(p, "digest")
		}
	}
	_, amd := digests["amd64"]
	_, arm := digests["arm64"]
	if err := check(len(digests) == 2 && amd && arm, "Wrong platform set"); err != nil {
		return nil, err
	}
	fields := []string{"image", "tag", "commit", "source", "created", "lock_sha256", "run_id"}
	if explicit {
		fields = append(fields, "run_attempt")
	}
	if _, present := receipt["bundle"]; present {
		fields = append(fields, "bundle")
	}
	for _, item := range platforms {
		p, ok := item.(map[string]any)
		if err := check(ok, "Platform provenance differs"); err != nil {
			return nil, err
		}
		for _, key := range fields {
			if want, present := receipt[key]; !present || !reflect.DeepEqual(p[key], want) {
				return nil, reasonError("Platform provenance differs")
			}
		}
		if err := check(p["smoked"] == true, "Platform smoke missing"); err != nil {
			return nil, err
		}
		for _, key := range []string{"image_id", "digest"} {
			if s, ok := text(p, key); !ok || !digestPattern.MatchString(s) {
				return nil, reasonError("Invalid platform digest")
			}
		}
	}
	return digests, nil
}

// annotation is the exact message of the immutable source tag.
func annotation(r *stagingRecord) string {
	return fmt.Sprintf("BundleDock %s/%s\n\nImage: %s:%s@%s\nIndex-Digest: %s\nSource: %s\nRevision: %s\nWorkflow: %s/actions/runs/%s/attempts/%d\n",
		r.bundle, r.tag, r.image, r.tag, r.digest, r.digest, r.source, r.commit, SourceURL, r.runID, r.attempt)
}

// verifyRegistry is verify_registry of scripts/bundle-git-tags.py with aliases:
// the version tag and every alias resolve to the recorded index, which holds
// exactly the linux amd64 and arm64 images whose labels match the record.
// It returns the platform manifest digests of the index.
func verifyRegistry(ctx context.Context, ev Evidence, r *stagingRecord, aliases []string) (map[string]string, error) {
	type document struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	raw, err := ev.ManifestRef(ctx, r.tag)
	if err != nil {
		return nil, err
	}
	if err := checkDigest(raw, r.digest); err != nil {
		return nil, err
	}
	var index document
	if err := unmarshal(raw, &index); err != nil {
		return nil, err
	}
	// Fetch through the immutable name too; the version must resolve to it.
	byDigest, err := ev.Manifest(ctx, r.digest)
	if err != nil {
		return nil, err
	}
	if err := checkDigest(byDigest, r.digest); err != nil {
		return nil, err
	}
	platforms := map[string]string{}
	for _, d := range index.Manifests {
		arch := d.Platform.Architecture
		if _, dup := platforms[arch]; d.Platform.OS != "linux" || (arch != "amd64" && arch != "arm64") || dup {
			return nil, reasonError("expected exactly linux amd64 and arm64")
		}
		platforms[arch] = d.Digest
		manifestRaw, err := ev.Manifest(ctx, d.Digest)
		if err != nil {
			return nil, err
		}
		if err := checkDigest(manifestRaw, d.Digest); err != nil {
			return nil, err
		}
		var manifest struct {
			Config struct {
				Digest string `json:"digest"`
			} `json:"config"`
		}
		if err := unmarshal(manifestRaw, &manifest); err != nil {
			return nil, err
		}
		configRaw, err := ev.Blob(ctx, manifest.Config.Digest)
		if err != nil {
			return nil, err
		}
		if err := checkDigest(configRaw, manifest.Config.Digest); err != nil {
			return nil, err
		}
		var config struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Config       struct {
				Labels map[string]string `json:"Labels"`
			} `json:"config"`
		}
		if err := unmarshal(configRaw, &config); err != nil {
			return nil, err
		}
		if err := check(config.OS == "linux" && config.Architecture == arch, "OCI config platform mismatch"); err != nil {
			return nil, err
		}
		if err := verifyLabels(r, config.Config.Labels); err != nil {
			return nil, err
		}
	}
	if err := check(len(platforms) == 2, "missing required architecture"); err != nil {
		return nil, err
	}
	for _, alias := range aliases {
		aliasRaw, err := ev.ManifestRef(ctx, alias)
		if err != nil {
			return nil, err
		}
		if err := checkDigest(aliasRaw, r.digest); err != nil {
			return nil, err
		}
	}
	return platforms, nil
}

// ProvePublication verifies a publication receipt (publication_proof of the
// script): receipt shape and provenance, the published source commit on main,
// the registry index, aliases and labels, the platform digests and the
// immutable source tag. On success it becomes the pruner's publication.
func (p *Pruner) ProvePublication(ctx context.Context, raw []byte, bundle string, env PublicationEnv) (*Publication, error) {
	b, err := FindBundle(bundle)
	if err != nil {
		return nil, reasonError("Unknown bundle")
	}
	var receipt map[string]any
	if err := json.Unmarshal(raw, &receipt); err != nil || receipt == nil {
		return nil, reasonError("Receipt must be an object")
	}
	digests, err := validateReceipt(receipt, b, env)
	if err != nil {
		return nil, err
	}
	str := func(key string) string { s, _ := text(receipt, key); return s }
	record := &stagingRecord{bundle: bundle, image: str("image"), tag: str("tag"), digest: str("digest"), source: str("source"),
		commit: str("commit"), runID: str("run_id")}
	record.runAttempt, record.hasAttempt = text(receipt, "run_attempt")
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	attempt, _ := strconv.Atoi(env.RunAttempt)
	record.attempt = attempt
	ev, err := p.evidenceFor(ctx, b.Repository)
	if err != nil {
		return nil, err
	}
	if p.Git == nil {
		return nil, fmt.Errorf("no Git verifier configured")
	}
	if err := p.Git.VerifySource(ctx, record.commit); err != nil {
		return nil, err
	}
	index, err := verifyRegistry(ctx, ev, record, b.Aliases)
	if err != nil {
		return nil, err
	}
	if err := check(reflect.DeepEqual(index, digests), "Published platform digests differ from receipt"); err != nil {
		return nil, err
	}
	found, err := p.Git.RemoteTag(ctx, bundle+"/"+record.tag, record.commit, annotation(record))
	if err != nil {
		return nil, err
	}
	if err := check(found, "Verified source tag missing"); err != nil {
		return nil, err
	}
	pub := &Publication{Bundle: bundle, Tag: record.tag, Commit: record.commit, RunID: record.runID, Attempt: attempt, Platforms: digests}
	p.Publication = pub
	return pub, nil
}

// stagingNames are the two architecture staging tags of the publication.
func (pub *Publication) stagingNames() []string {
	return []string{"staging-" + pub.Tag + "-amd64", "staging-" + pub.Tag + "-arm64"}
}

func (pub *Publication) names(name string) bool {
	for _, n := range pub.stagingNames() {
		if n == name {
			return true
		}
	}
	return false
}
