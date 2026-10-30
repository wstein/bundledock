package toolpublish

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"bundledock/internal/testdir"
)

// requireNativeLinuxBuild skips tests that drive Adapter.Build through its
// fake Docker: Build refuses to run unless the host is Linux, by design, so
// on other hosts these tests would fail on the guard rather than on their
// subject.
func requireNativeLinuxBuild(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("Adapter.Build requires a native Linux host (GOOS=%s)", runtime.GOOS)
	}
}

type fakeDocker struct {
	arch, id, digest, indexDigest     string
	calls                             [][]string
	labels                            map[string]string
	remote                            map[string]any
	failSmoke, failPull, corruptIndex bool
	local                             string
}

func fake(arch string) *fakeDocker {
	letter := "a"
	if arch == "arm64" {
		letter = "b"
	}
	return &fakeDocker{arch: arch, id: "sha256:" + strings.Repeat(letter, 64), digest: "sha256:" + strings.Repeat(letter, 63) + "c", indexDigest: "sha256:" + strings.Repeat("d", 64), remote: map[string]any{}}
}

func (f *fakeDocker) adapter() Adapter {
	return Adapter{Run: f.run, Architecture: f.arch, TagAbsent: func(image, tag string) error {
		f.calls = append(f.calls, []string{"check-tag-absent", image, tag})
		if _, exists := f.remote[image+":"+tag]; exists {
			return errors.New("version already exists")
		}
		return nil
	}}
}

func prefix(args []string, want ...string) bool {
	return len(args) >= len(want) && reflect.DeepEqual(args[:len(want)], want)
}

func (f *fakeDocker) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case args[0] == "sh":
		if f.failSmoke {
			return nil, errors.New("native smoke failed")
		}
	case prefix(args, "docker", "pull"):
		if f.failPull {
			return nil, errors.New("consumer pull failed")
		}
	case prefix(args, "docker", "build"):
		f.labels = map[string]string{}
		for i, arg := range args {
			if arg == "--label" {
				k, v, _ := strings.Cut(args[i+1], "=")
				f.labels[k] = v
			}
			if arg == "-t" {
				f.local = args[i+1]
			}
		}
	case prefix(args, "docker", "image", "inspect"):
		if args[3] != f.local && args[3] != f.id {
			return nil, fmt.Errorf("unknown local image %q", args[3])
		}
		return json.Marshal([]any{map[string]any{"Id": f.id, "Architecture": f.arch, "Os": "linux", "Config": map[string]any{"Labels": f.labels}}})
	case prefix(args, "docker", "push"):
		image, _, _ := strings.Cut(args[len(args)-1], ":staging-")
		f.remote[image+"@"+f.digest] = map[string]any{"config": map[string]string{"digest": f.id}}
	case prefix(args, "docker", "buildx", "imagetools", "create"):
		image, _, _ := strings.Cut(args[5], ":")
		if len(args) == 7 {
			f.remote[image+"@"+f.indexDigest] = f.remote[args[6]]
			return nil, nil
		}
		var manifests []any
		for _, ref := range args[6:] {
			_, dg, _ := strings.Cut(ref, "@")
			arch := "amd64"
			if strings.Contains(dg, strings.Repeat("b", 32)) {
				arch = "arm64"
			}
			if f.corruptIndex {
				dg = "sha256:" + strings.Repeat("e", 64)
			}
			manifests = append(manifests, map[string]any{"digest": dg, "platform": map[string]string{"os": "linux", "architecture": arch}})
		}
		f.remote[image+"@"+f.indexDigest] = map[string]any{"manifests": manifests}
	case prefix(args, "docker", "buildx", "imagetools", "inspect"):
		ref := args[len(args)-1]
		if args[4] == "--raw" {
			value, ok := f.remote[ref]
			if !ok {
				return nil, fmt.Errorf("unknown registry ref %s", ref)
			}
			return json.Marshal(value)
		}
		dg := f.indexDigest
		if strings.Contains(ref, ":staging-") {
			dg = f.digest
		}
		return json.Marshal(map[string]string{"digest": dg})
	}
	return nil, nil
}

func fixture(t *testing.T) (Metadata, string) {
	t.Helper()
	lock := filepath.Join(testdir.New(t), "packages.lock.json")
	if err := os.WriteFile(lock, []byte(`{"schema":2,"platforms":{"amd64":{"gh":{"version":"v1.2.3","url":"https://github.com/example/amd64","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","verification":"tls-download-sha256"}},"arm64":{"gh":{"version":"v1.2.3","url":"https://github.com/example/arm64","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","verification":"tls-download-sha256"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewMetadata(lock, "quay.io/example/toolbox", "2026.10.10-b12.1", strings.Repeat("f", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044")
	if err != nil {
		t.Fatal(err)
	}
	return m, lock
}

func buildFixture(t *testing.T, m Metadata, lock, arch string) (*fakeDocker, Receipt) {
	t.Helper()
	requireNativeLinuxBuild(t)
	f := fake(arch)
	r, err := (Adapter{Run: f.run, Architecture: arch}).Build(m, lock, arch, "consumer:test")
	if err != nil {
		t.Fatal(err)
	}
	return f, r
}

func TestBuildSmokeThenPushExactImage(t *testing.T) {
	m, lock := fixture(t)
	f, r := buildFixture(t, m, lock, "amd64")
	pull, smoke := -1, -1
	for i, c := range f.calls {
		if reflect.DeepEqual(c, []string{"docker", "pull", "consumer:test"}) {
			pull = i
		}
		if reflect.DeepEqual(c, []string{"sh", "bundles/toolbox/scripts/smoke.sh", f.id, "consumer:test"}) {
			smoke = i
		}
		if prefix(c, "docker", "push") {
			t.Fatal("build pushed before explicit publication")
		}
	}
	if pull < 0 || smoke <= pull {
		t.Fatalf("consumer must be pulled before exact-ID smoke: %v", f.calls)
	}
	p, err := f.adapter().Push(m, lock, r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Digest != f.digest || p.Digest == p.ImageID {
		t.Fatalf("registry digest must differ from local config ID: %+v", p)
	}
	builds, exactTag := 0, false
	for _, c := range f.calls {
		if prefix(c, "docker", "build") {
			builds++
		}
		if reflect.DeepEqual(c, []string{"docker", "tag", f.id, m.Image + ":staging-" + m.Tag + "-amd64"}) {
			exactTag = true
		}
	}
	if builds != 1 || !exactTag {
		t.Fatalf("must push original smoked image without rebuild: %v", f.calls)
	}
	if f.labels["io.bundledock.github.run-id"] != m.RunID {
		t.Fatal("missing full GitHub run ID provenance")
	}
	b, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ToolDigest(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.labels[platformsLabel] != want {
		t.Fatal("missing durable published tool/asset baseline")
	}
	if f.labels["io.bundledock.tools.versions"] != `{"gh":"v1.2.3"}` || f.labels["org.opencontainers.image.title"] != "bundledock-toolbox" {
		t.Fatal("missing tool versions/title provenance")
	}
}

func TestBuildFailureGates(t *testing.T) {
	for _, scenario := range []string{"smoke", "pull", "native", "lock"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "smoke" || scenario == "pull" {
				// These must fail inside Build after the platform guard; on a
				// non-Linux host they would pass vacuously on the guard.
				requireNativeLinuxBuild(t)
			}
			m, lock := fixture(t)
			f := fake("amd64")
			a := Adapter{Run: f.run, Architecture: "amd64"}
			if scenario == "smoke" {
				f.failSmoke = true
			}
			if scenario == "pull" {
				f.failPull = true
			}
			if scenario == "native" {
				a.Architecture = "arm64"
			}
			if scenario == "lock" {
				if err := os.WriteFile(lock, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r, err := a.Build(m, lock, "amd64", "consumer:test")
			if err == nil || r.Smoked {
				t.Fatal("failed prerequisite produced smoke receipt")
			}
			for _, c := range f.calls {
				if prefix(c, "docker", "push") {
					t.Fatal("failed build published")
				}
				if scenario == "pull" && c[0] == "sh" {
					t.Fatal("failed consumer pull ran smoke")
				}
			}
			if (scenario == "native" || scenario == "lock") && len(f.calls) != 0 {
				t.Fatal("invalid input ran Docker")
			}
		})
	}
}

func TestReceiptAndLabelsBindBeforePush(t *testing.T) {
	m, lock := fixture(t)
	f, r := buildFixture(t, m, lock, "amd64")
	for _, field := range []string{"lock", "architecture", "image", "smoke", "revision"} {
		bad := r
		switch field {
		case "lock":
			bad.LockSHA256 = strings.Repeat("0", 64)
		case "architecture":
			bad.Architecture = "arm64"
		case "image":
			bad.ImageID = "sha256:" + strings.Repeat("e", 64)
		case "smoke":
			bad.Smoked = false
		case "revision":
			bad.Commit = strings.Repeat("e", 40)
		}
		if _, err := f.adapter().Push(m, lock, bad); err == nil {
			t.Fatalf("accepted invalid %s receipt", field)
		}
	}
	f.labels["io.bundledock.tools.lock-sha256"] = "wrong"
	if _, err := f.adapter().Push(m, lock, r); err == nil {
		t.Fatal("accepted wrong image labels")
	}
	for _, c := range f.calls {
		if prefix(c, "docker", "push") {
			t.Fatal("invalid receipt published")
		}
	}
}

func publishedFixtures(t *testing.T, m Metadata, lock string) ([]Receipt, *fakeDocker) {
	t.Helper()
	var receipts []Receipt
	remote := map[string]any{}
	for _, arch := range []string{"amd64", "arm64"} {
		f, r := buildFixture(t, m, lock, arch)
		p, err := f.adapter().Push(m, lock, r)
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, p)
		for k, v := range f.remote {
			remote[k] = v
		}
	}
	f := fake("amd64")
	f.remote = remote
	return receipts, f
}

func TestPromotionRequiresBothArchitecturesAndRegistryBinding(t *testing.T) {
	m, lock := fixture(t)
	receipts, f := publishedFixtures(t, m, lock)
	a := f.adapter()
	badLock := receipts[1]
	badLock.LockSHA256 = strings.Repeat("0", 64)
	badCommit := receipts[1]
	badCommit.Commit = strings.Repeat("e", 40)
	for _, bad := range [][]Receipt{receipts[:1], {receipts[0], receipts[0]}, {receipts[0], badLock}, {receipts[0], badCommit}} {
		if _, err := a.Promote(m, lock, bad); err == nil {
			t.Fatal("promoted incomplete or mismatched native evidence")
		}
	}
	key := m.Image + "@" + receipts[1].Digest
	original := f.remote[key]
	f.remote[key] = map[string]any{"config": map[string]string{"digest": "sha256:" + strings.Repeat("e", 64)}}
	if _, err := a.Promote(m, lock, receipts); err == nil {
		t.Fatal("promoted untested registry config")
	}
	f.remote[key] = original
	for _, c := range f.calls {
		if prefix(c, "docker", "buildx", "imagetools", "create") {
			t.Fatal("invalid evidence created a manifest")
		}
	}
	result, err := a.Promote(m, lock, receipts)
	if err != nil {
		t.Fatal(err)
	}
	var creates [][]string
	for _, c := range f.calls {
		if prefix(c, "docker", "buildx", "imagetools", "create") {
			creates = append(creates, c)
		}
	}
	if len(creates) != 3 || creates[0][5] != m.Image+":"+m.Tag || creates[1][5] != m.Image+":stable" || creates[1][6] != result.Reference || creates[2][5] != m.Image+":latest" || creates[2][6] != result.Reference {
		t.Fatalf("incorrect promotion: %+v %v", result, creates)
	}
}

func TestBadPublishedIndexPreventsStable(t *testing.T) {
	m, lock := fixture(t)
	receipts, f := publishedFixtures(t, m, lock)
	f.corruptIndex = true
	if _, err := f.adapter().Promote(m, lock, receipts); err == nil {
		t.Fatal("invalid published platform index promoted")
	}
	for _, c := range f.calls {
		if prefix(c, "docker", "buildx", "imagetools", "create") && (c[5] == m.Image+":stable" || c[5] == m.Image+":latest") {
			t.Fatal("invalid index updated a mutable alias")
		}
	}
}

func TestDestinationValidation(t *testing.T) {
	_, lock := fixture(t)
	for _, image := range []string{"", "docker.io/example/toolbox", "quay.io/example/tools", "quay.io/example/toolbox:stable", "quay.io/example/toolbox;echo hi", "quay.io/example/workharbor-tools", "quay.io/example/bundledock-tools", "quay.io/example/bundledock-toolbox", "quay.io/example/toolbox/", "https://quay.io/example/toolbox", "quay.io/example/toolbox@sha256:" + strings.Repeat("a", 64)} {
		if _, err := NewMetadata(lock, image, "2026.10.10-b12.1", strings.Repeat("f", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044"); err == nil {
			t.Fatalf("accepted invalid destination %q", image)
		}
	}
}

func TestBundleDockDestinationAccepted(t *testing.T) {
	_, lock := fixture(t)
	if _, err := NewMetadata(lock, "quay.io/example/toolbox", "2026.10.10-b12.1", strings.Repeat("f", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044"); err != nil {
		t.Fatalf("rejected BundleDock destination: %v", err)
	}
}

func TestShortVersionTags(t *testing.T) {
	lock := filepath.Join(testdir.New(t), "lock.json")
	if err := os.WriteFile(lock, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"2026.10.10-b12.4", "2026.10.10-b13.4", "2026.10.10-b12.5"} {
		if _, err := NewMetadata(lock, "quay.io/example/toolbox", tag, strings.Repeat("f", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044"); err != nil {
			t.Errorf("valid distinct tag %s: %v", tag, err)
		}
	}
	for _, tag := range []string{"2026.10.10-38069722044-4", "2026.10.10-b0.4", "2026.10.10-b12.0", "2026.10.10-b01.4", "2026.10.10-b12.-1", "2026.10.10-b12.abc"} {
		if _, err := NewMetadata(lock, "quay.io/example/toolbox", tag, strings.Repeat("f", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044"); err == nil {
			t.Errorf("accepted invalid tag %s", tag)
		}
	}
}

func TestExistingVersionPreventsRegistryWrites(t *testing.T) {
	for _, operation := range []string{"push", "promote"} {
		t.Run(operation, func(t *testing.T) {
			m, lock := fixture(t)
			var f *fakeDocker
			var receipt Receipt
			var receipts []Receipt
			if operation == "push" {
				f, receipt = buildFixture(t, m, lock, "amd64")
			} else {
				receipts, f = publishedFixtures(t, m, lock)
			}
			f.remote[m.Image+":"+m.Tag] = map[string]string{"digest": f.indexDigest}
			f.calls = nil
			var err error
			if operation == "push" {
				_, err = f.adapter().Push(m, lock, receipt)
			} else {
				_, err = f.adapter().Promote(m, lock, receipts)
			}
			if err == nil {
				t.Error("existing immutable version accepted")
			}
			for _, c := range f.calls {
				if prefix(c, "docker", "tag") || prefix(c, "docker", "push") || prefix(c, "docker", "buildx", "imagetools", "create") {
					t.Errorf("existing version allowed registry mutation: %v", c)
				}
			}
		})
	}
}

func TestUnavailableVersionCheckPreventsRegistryWrites(t *testing.T) {
	for _, operation := range []string{"push", "promote"} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/missing=%t", operation, missing), func(t *testing.T) {
				m, lock := fixture(t)
				var f *fakeDocker
				var receipt Receipt
				var receipts []Receipt
				if operation == "push" {
					f, receipt = buildFixture(t, m, lock, "amd64")
				} else {
					receipts, f = publishedFixtures(t, m, lock)
				}
				a := f.adapter()
				a.TagAbsent = func(string, string) error { return errors.New("registry unavailable") }
				if missing {
					a.TagAbsent = nil
				}
				f.calls = nil
				var err error
				if operation == "push" {
					_, err = a.Push(m, lock, receipt)
				} else {
					_, err = a.Promote(m, lock, receipts)
				}
				if err == nil {
					t.Error("unavailable version check accepted")
				}
				for _, c := range f.calls {
					if prefix(c, "docker", "tag") || prefix(c, "docker", "push") || prefix(c, "docker", "buildx", "imagetools", "create") {
						t.Errorf("failed check allowed mutation: %v", c)
					}
				}
			})
		}
	}
}

func TestPromotionPublishesLatestAtTheVerifiedDigest(t *testing.T) {
	m, lock := fixture(t)
	receipts, f := publishedFixtures(t, m, lock)
	// Mutable aliases can already exist; only the dated version is collision-gated.
	f.remote[m.Image+":stable"] = map[string]string{"digest": "old"}
	f.remote[m.Image+":latest"] = map[string]string{"digest": "old"}
	result, err := f.adapter().Promote(m, lock, receipts)
	if err != nil {
		t.Fatal(err)
	}
	var creates [][]string
	for _, c := range f.calls {
		if prefix(c, "docker", "buildx", "imagetools", "create") {
			creates = append(creates, c)
		}
	}
	if len(creates) != 3 {
		t.Fatalf("expected version, stable and latest: %v", creates)
	}
	for i, alias := range []string{"stable", "latest"} {
		if !reflect.DeepEqual(creates[i+1], []string{"docker", "buildx", "imagetools", "create", "--tag", m.Image + ":" + alias, result.Reference}) {
			t.Errorf("alias %s differs from verified digest: %v", alias, creates[i+1])
		}
	}
}

func TestAliasFailureDoesNotReportSuccessfulPublication(t *testing.T) {
	for _, alias := range []string{"stable", "latest"} {
		for _, failure := range []string{"create", "inspect", "digest"} {
			t.Run(alias+"/"+failure, func(t *testing.T) {
				m, lock := fixture(t)
				receipts, f := publishedFixtures(t, m, lock)
				a := f.adapter()
				a.Run = func(args ...string) ([]byte, error) {
					if prefix(args, "docker", "buildx", "imagetools", "create") && args[5] == m.Image+":"+alias && failure == "create" {
						return nil, errors.New("alias write failed")
					}
					if prefix(args, "docker", "buildx", "imagetools", "inspect") && args[len(args)-1] == m.Image+":"+alias {
						if failure == "inspect" {
							return nil, errors.New("alias verification failed")
						}
						if failure == "digest" {
							return json.Marshal(map[string]string{"digest": "sha256:" + strings.Repeat("e", 64)})
						}
					}
					return f.run(args...)
				}
				result, err := a.Promote(m, lock, receipts)
				if err == nil || result.Reference != "" {
					t.Fatalf("alias failure reported successful publication: %+v %v", result, err)
				}
			})
		}
	}
}

func TestCodeServerBuildUsesDedicatedCarrierAndSmoke(t *testing.T) {
	requireNativeLinuxBuild(t)
	lock := "../../bundles/code-server/packages.lock.json"
	m, err := NewMetadata(lock, "quay.io/example/code-server", "2026.10.10-b12.1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "123")
	if err != nil {
		t.Fatal(err)
	}
	f := fake("amd64")
	r, err := f.adapter().Build(m, lock, "amd64", "consumer:test")
	if err != nil {
		t.Fatal(err)
	}
	build, smoke := false, false
	for _, args := range f.calls {
		if prefix(args, "docker", "build") {
			build = true
			if args[6] != "bundles/code-server/Dockerfile" || !strings.HasPrefix(args[8], "bundledock-code-server:build-") {
				t.Fatalf("wrong build %v", args)
			}
		}
		if prefix(args, "sh") {
			smoke = true
			if !reflect.DeepEqual(args, []string{"sh", "bundles/code-server/scripts/smoke.sh", r.ImageID, "consumer:test"}) {
				t.Fatalf("wrong smoke %v", args)
			}
		}
	}
	if !build || !smoke || f.labels["org.opencontainers.image.title"] != "bundledock-code-server" {
		t.Fatal("missing dedicated native path")
	}
	f.failSmoke = true
	if _, err := f.adapter().Build(m, lock, "amd64", "consumer:test"); err == nil {
		t.Fatal("accepted failed code-server smoke")
	}
}
