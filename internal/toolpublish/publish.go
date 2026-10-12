// Package toolpublish gates OCI publication on native smoke-test evidence.
package toolpublish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type Metadata struct {
	Image      string `json:"image"`
	RunID      string `json:"run_id"`
	RunAttempt string `json:"run_attempt,omitempty"`
	Tag        string `json:"tag"`
	Commit     string `json:"commit"`
	Source     string `json:"source"`
	Created    string `json:"created"`
	LockSHA256 string `json:"lock_sha256"`
}

type Receipt struct {
	Metadata
	Architecture string `json:"architecture"`
	ImageID      string `json:"image_id"`
	Smoked       bool   `json:"smoked"`
	Digest       string `json:"digest,omitempty"`
}

type Publication struct {
	Metadata
	Digest    string    `json:"digest"`
	Reference string    `json:"reference"`
	Platforms []Receipt `json:"platforms"`
}

type Adapter struct {
	Run          func(...string) ([]byte, error)
	Architecture string
	TagAbsent    func(image, tag string) error
}

var imagePattern = regexp.MustCompile(`^quay\.io/[a-z0-9]+(?:[._-][a-z0-9]+)*/(toolbox|code-server|node|bun|codex|agy|claude)$`)
var tagPattern = regexp.MustCompile(`^(?:\d{4}\.\d{2}\.\d{2}-b[1-9]\d*\.[1-9]\d*|(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)-b[1-9]\d*)$`)
var runIDPattern = regexp.MustCompile(`^[1-9]\d*$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// RepositoryImage accepts a Quay namespace prefix with one optional trailing
// slash. The publication repository is always toolbox; tags come from the run.
func RepositoryImage(prefix string) (string, error) {
	image := strings.TrimSuffix(prefix, "/") + "/toolbox"
	if !imagePattern.MatchString(image) {
		return "", fmt.Errorf("REPOSITORY_URL must be quay.io/<namespace> with an optional trailing slash")
	}
	return image, nil
}

func HashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func NewMetadata(lock, image, tag, commit, source, created, runID string, attempt ...string) (Metadata, error) {
	m := Metadata{Image: image, Tag: tag, Commit: commit, Source: source, Created: created, RunID: runID}
	if !imagePattern.MatchString(image) {
		return m, fmt.Errorf("publication image must be quay.io/<namespace>/{toolbox,code-server,node,bun,codex,agy,claude}")
	}
	if !tagPattern.MatchString(tag) || !commitPattern.MatchString(commit) || !runIDPattern.MatchString(runID) {
		return m, fmt.Errorf("invalid date/run/attempt tag, source commit or run ID")
	}
	if singletonTagPattern.MatchString(tag) {
		if bundleForImage(image).Name == "toolbox" || len(attempt) != 1 || !runIDPattern.MatchString(attempt[0]) {
			return m, fmt.Errorf("singleton tag requires workflow attempt")
		}
		m.RunAttempt = attempt[0]
		data, err := os.ReadFile(lock)
		if err != nil {
			return m, err
		}
		version, err := SingletonVersion(data, bundleForImage(image).Name)
		if err != nil || !strings.HasPrefix(tag, version+"-b") {
			return m, fmt.Errorf("version tag differs from frozen upstream lock")
		}
	}
	if _, err := time.Parse("2006-01-02T15:04:05Z", created); err != nil {
		return m, err
	}
	if len(source) < 9 || source[:8] != "https://" {
		return m, fmt.Errorf("HTTPS source provenance required")
	}
	var err error
	m.LockSHA256, err = HashFile(lock)
	return m, err
}

func metadataAttempt(m Metadata) []string {
	if m.RunAttempt != "" {
		return []string{m.RunAttempt}
	}
	return nil
}

func validate(m Metadata, lock string) error {
	expected, err := NewMetadata(lock, m.Image, m.Tag, m.Commit, m.Source, m.Created, m.RunID, metadataAttempt(m)...)
	if err != nil {
		return err
	}
	if expected != m {
		return fmt.Errorf("resolved lock/provenance mismatch")
	}
	return nil
}

func labels(m Metadata, lock, arch string) (map[string]string, error) {
	var pins struct {
		Platforms map[string]map[string]struct {
			Version string `json:"version"`
		} `json:"platforms"`
	}
	b, err := os.ReadFile(lock)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &pins); err != nil {
		return nil, err
	}
	versions := map[string]string{}
	for name, pin := range pins.Platforms[arch] {
		versions[name] = pin.Version
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("lock has no tools for %s", arch)
	}
	encoded, err := json.Marshal(versions)
	if err != nil {
		return nil, err
	}
	semantic, err := ToolDigest(b)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"org.opencontainers.image.title": "bundledock-" + bundleForImage(m.Image).Name, "org.opencontainers.image.source": m.Source,
		"org.opencontainers.image.revision": m.Commit, "org.opencontainers.image.created": m.Created,
		"org.opencontainers.image.version": m.Tag, "io.bundledock.tools.lock-sha256": m.LockSHA256,
		"io.bundledock.tools.versions": string(encoded), "io.bundledock.github.run-id": m.RunID,
		"io.bundledock.github.run-attempt": m.RunAttempt,
		platformsLabel:                     semantic,
	}, nil
}

func (a Adapter) json(out any, args ...string) error {
	b, err := a.Run(args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (a Adapter) inspectLocal(m Metadata, lock, arch, ref string) (string, error) {
	var images []struct {
		ID           string `json:"Id"`
		Architecture string
		Os           string
		Config       struct{ Labels map[string]string }
	}
	if err := a.json(&images, "docker", "image", "inspect", ref); err != nil {
		return "", err
	}
	if len(images) != 1 {
		return "", fmt.Errorf("expected exactly one local image")
	}
	info := images[0]
	if info.Architecture != arch || info.Os != "linux" {
		return "", fmt.Errorf("image platform mismatch")
	}
	want, err := labels(m, lock, arch)
	if err != nil {
		return "", err
	}
	for k, v := range want {
		if info.Config.Labels[k] != v {
			return "", fmt.Errorf("image provenance label mismatch: %s", k)
		}
	}
	if !digestPattern.MatchString(info.ID) {
		return "", fmt.Errorf("invalid local image ID")
	}
	return info.ID, nil
}

// Smoke pulls the consumer before smoke.sh disables networking; the tool image
// argument is an immutable local config ID, never a mutable build tag.
func (a Adapter) Smoke(id, consumer string) error {
	return a.smokeBundle(id, consumer, "bundles/toolbox/smoke.sh")
}
func (a Adapter) smokeBundle(id, consumer, script string) error {
	if _, err := a.Run("docker", "pull", consumer); err != nil {
		return err
	}
	_, err := a.Run("sh", script, id, consumer)
	return err
}

func (a Adapter) Build(m Metadata, lock, arch, consumer string) (Receipt, error) {
	var r Receipt
	if err := validate(m, lock); err != nil {
		return r, err
	}
	native := a.Architecture
	if native == "" {
		native = runtime.GOARCH
	}
	if runtime.GOOS != "linux" || native != arch || (arch != "amd64" && arch != "arm64") {
		return r, fmt.Errorf("native Linux architecture required")
	}
	bundle := bundleForImage(m.Image)
	args := []string{"docker", "build", "--pull", "--platform", "linux/" + arch, "-f", bundle.Directory + "/Dockerfile", "-t", "bundledock-" + bundle.Name + ":build-" + m.Tag + "-" + arch}
	imageLabels, err := labels(m, lock, arch)
	if err != nil {
		return r, err
	}
	for k, v := range imageLabels {
		args = append(args, "--label", k+"="+v)
	}
	if _, err = a.Run(append(args, ".")...); err != nil {
		return r, err
	}
	id, err := a.inspectLocal(m, lock, arch, args[8])
	if err != nil {
		return r, err
	}
	if err = a.smokeBundle(id, consumer, bundle.Directory+"/smoke.sh"); err != nil {
		return r, err
	}
	return Receipt{Metadata: m, Architecture: arch, ImageID: id, Smoked: true}, nil
}

func validateReceipt(m Metadata, r Receipt) error {
	if r.Metadata != m || !r.Smoked || !digestPattern.MatchString(r.ImageID) || (r.Architecture != "amd64" && r.Architecture != "arm64") {
		return fmt.Errorf("receipt provenance, platform or smoke mismatch")
	}
	return nil
}

func (a Adapter) remoteDigest(ref string) (string, error) {
	var descriptor struct {
		Digest string `json:"digest"`
	}
	// Registry manifest digests differ from Docker's local config image IDs.
	if err := a.json(&descriptor, "docker", "buildx", "imagetools", "inspect", "--format", "{{json .Manifest}}", ref); err != nil {
		return "", err
	}
	if !digestPattern.MatchString(descriptor.Digest) {
		return "", fmt.Errorf("invalid registry manifest digest")
	}
	return descriptor.Digest, nil
}

func (a Adapter) verifyRemote(m Metadata, r Receipt) (string, error) {
	if !digestPattern.MatchString(r.Digest) {
		return "", fmt.Errorf("invalid receipt registry digest")
	}
	ref := m.Image + "@" + r.Digest
	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := a.json(&manifest, "docker", "buildx", "imagetools", "inspect", "--raw", ref); err != nil {
		return "", err
	}
	if manifest.Config.Digest != r.ImageID {
		return "", fmt.Errorf("registry manifest does not reference the smoked image config")
	}
	return ref, nil
}

func (a Adapter) checkVersion(m Metadata) error {
	if a.TagAbsent == nil {
		return fmt.Errorf("registry version absence check required")
	}
	return a.TagAbsent(m.Image, m.Tag)
}

func (a Adapter) Push(m Metadata, lock string, r Receipt) (Receipt, error) {
	var empty Receipt
	if err := validate(m, lock); err != nil {
		return empty, err
	}
	if err := validateReceipt(m, r); err != nil {
		return empty, err
	}
	id, err := a.inspectLocal(m, lock, r.Architecture, r.ImageID)
	if err != nil {
		return empty, err
	}
	if id != r.ImageID {
		return empty, fmt.Errorf("smoked local image changed")
	}
	if err = a.checkVersion(m); err != nil {
		return empty, err
	}
	staging := m.Image + ":staging-" + m.Tag + "-" + r.Architecture
	if _, err = a.Run("docker", "tag", id, staging); err != nil {
		return empty, err
	}
	if _, err = a.Run("docker", "push", staging); err != nil {
		return empty, err
	}
	r.Digest, err = a.remoteDigest(staging)
	if err != nil {
		return empty, err
	}
	if _, err = a.verifyRemote(m, r); err != nil {
		return empty, err
	}
	return r, nil
}

func (a Adapter) Promote(m Metadata, lock string, receipts []Receipt) (Publication, error) {
	var result Publication
	if err := validate(m, lock); err != nil {
		return result, err
	}
	seen := map[string]bool{}
	if len(receipts) != 2 {
		return result, fmt.Errorf("both native architectures must pass before promotion")
	}
	for _, r := range receipts {
		if err := validateReceipt(m, r); err != nil {
			return result, err
		}
		seen[r.Architecture] = true
	}
	if !seen["amd64"] || !seen["arm64"] {
		return result, fmt.Errorf("both native architectures must pass before promotion")
	}
	args := []string{"docker", "buildx", "imagetools", "create", "--tag", m.Image + ":" + m.Tag}
	for _, r := range receipts {
		ref, err := a.verifyRemote(m, r)
		if err != nil {
			return result, err
		}
		args = append(args, ref)
	}
	if err := a.checkVersion(m); err != nil {
		return result, err
	}
	if _, err := a.Run(args...); err != nil {
		return result, err
	}
	versionDigest, err := a.remoteDigest(m.Image + ":" + m.Tag)
	if err != nil {
		return result, err
	}
	var index struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	ref := m.Image + "@" + versionDigest
	if err = a.json(&index, "docker", "buildx", "imagetools", "inspect", "--raw", ref); err != nil {
		return result, err
	}
	expected := map[string]string{}
	for _, r := range receipts {
		expected[r.Architecture] = r.Digest
	}
	if len(index.Manifests) != 2 {
		return result, fmt.Errorf("published index must contain exactly two tested platforms")
	}
	for _, manifest := range index.Manifests {
		if manifest.Platform.OS != "linux" || expected[manifest.Platform.Architecture] != manifest.Digest {
			return result, fmt.Errorf("published index contains untested platform digest")
		}
		delete(expected, manifest.Platform.Architecture)
	}
	if len(expected) != 0 {
		return result, fmt.Errorf("published index omits tested platform")
	}

	// Alias updates are sequential; a later failure cannot roll back earlier tags.
	for _, alias := range []string{"stable", "latest"} {
		if _, err = a.Run("docker", "buildx", "imagetools", "create", "--tag", m.Image+":"+alias, ref); err != nil {
			return result, fmt.Errorf("update %s alias: %w", alias, err)
		}
		aliasDigest, inspectErr := a.remoteDigest(m.Image + ":" + alias)
		if inspectErr != nil {
			return result, fmt.Errorf("verify %s alias: %w", alias, inspectErr)
		}
		if aliasDigest != versionDigest {
			return result, fmt.Errorf("%s registry digest differs from versioned index", alias)
		}
	}

	return Publication{Metadata: m, Digest: versionDigest, Reference: ref, Platforms: receipts}, nil
}
