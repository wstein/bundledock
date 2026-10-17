package releaseworkflow

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the BundleDock checkout holding this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func runIn(t *testing.T, dir string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func mustRun(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	out, err := runIn(t, dir, env, name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return out
}

func script(t *testing.T, name string) string {
	return filepath.Join(repoRoot(t), ".github", "scripts", name)
}

func TestVerifyVersionIsStrict(t *testing.T) {
	good := []string{"0.1.0", "1.2.3", "10.20.30"}
	bad := []string{"", "v0.1.0", "01.2.3", "1.2", "1.2.3.4", "1.2.3-rc1", "1.2.3\n4.5.6", "bdock/v1.2.3", "1.2.3 ", "-1.2.3"}
	for _, v := range good {
		if out, err := runIn(t, repoRoot(t), nil, "sh", script(t, "release-verify.sh"), "version", v); err != nil {
			t.Errorf("version %q rejected: %s", v, out)
		}
	}
	for _, v := range bad {
		if _, err := runIn(t, repoRoot(t), nil, "sh", script(t, "release-verify.sh"), "version", v); err == nil {
			t.Errorf("version %q accepted", v)
		}
	}
}

func TestVerifyDryVersionNeedsSuffix(t *testing.T) {
	for v, ok := range map[string]bool{"0.0.0-dryrun": true, "1.2.3-try.1": true, "0.1.0": false, "0.1.0-": false, "v0.1.0-x": false, "1.2.3-a b": false, "1.2.3-x\n1.2.3-y": false} {
		_, err := runIn(t, repoRoot(t), nil, "sh", script(t, "release-verify.sh"), "dryversion", v)
		if (err == nil) != ok {
			t.Errorf("dryversion %q: accepted=%v, want %v", v, err == nil, ok)
		}
	}
}

func TestVerifyTagNamespace(t *testing.T) {
	good := map[string]string{"bdock/v0.1.0": "0.1.0", "bdock/v12.0.3": "12.0.3"}
	for tag, want := range good {
		out, err := runIn(t, repoRoot(t), nil, "sh", script(t, "release-verify.sh"), "tag-version", tag)
		if err != nil || strings.TrimSpace(out) != want {
			t.Errorf("tag-version %q = %q, %v; want %q", tag, out, err, want)
		}
	}
	// Image tags, bare version tags and pre-releases are never CLI release tags.
	for _, tag := range []string{"v0.1.0", "bdock/0.1.0", "bdock/v0.1", "bdock/v0.1.0-rc1", "toolbox/2026.10.15-b98.1", "opentofu/1.13.1-b1", "bdock/v01.0.0", "xbdock/v0.1.0", "bdock/v0.1.0\nbdock/v0.1.1", ""} {
		if _, err := runIn(t, repoRoot(t), nil, "sh", script(t, "release-verify.sh"), "tag-version", tag); err == nil {
			t.Errorf("tag-version %q accepted", tag)
		}
	}
}

// gitRepo creates a repository with a main branch and one commit; it returns its path.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	env := []string{"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
	t.Setenv("GIT_AUTHOR_NAME", "t")
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	mustRun(t, dir, nil, "git", "init", "-q", "-b", "main")
	commit(t, dir, "first")
	return dir
}

func commit(t *testing.T, dir, subject string) string {
	t.Helper()
	mustRun(t, dir, nil, "git", "commit", "-q", "--allow-empty", "-m", subject)
	return strings.TrimSpace(mustRun(t, dir, nil, "git", "rev-parse", "HEAD"))
}

func TestVerifyTagChecksAnnotationAndMain(t *testing.T) {
	dir := gitRepo(t)
	main := commit(t, dir, "second")
	verify := func(tag string) (string, error) {
		return runIn(t, dir, nil, "sh", script(t, "release-verify.sh"), "tag", tag, "main")
	}
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "release", "bdock/v0.1.0")
	if out, err := verify("bdock/v0.1.0"); err != nil || strings.TrimSpace(out) != main {
		t.Fatalf("annotated tag on main = %q, %v", out, err)
	}
	mustRun(t, dir, nil, "git", "tag", "bdock/v0.1.1")
	if _, err := verify("bdock/v0.1.1"); err == nil {
		t.Error("lightweight tag accepted")
	}
	if _, err := verify("bdock/v0.9.9"); err == nil {
		t.Error("missing tag accepted")
	}
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "image", "toolbox/2026.10.15-b98.1")
	if _, err := verify("toolbox/2026.10.15-b98.1"); err == nil {
		t.Error("image tag accepted as CLI release")
	}
	mustRun(t, dir, nil, "git", "checkout", "-q", "-b", "side")
	commit(t, dir, "unreviewed")
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "release", "bdock/v0.2.0")
	if out, err := verify("bdock/v0.2.0"); err == nil {
		t.Errorf("tag on a commit outside main accepted: %s", out)
	}
}

func TestReleaseNotesNameBaselineAndFacts(t *testing.T) {
	dir := gitRepo(t)
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "image", "toolbox/2026.10.15-b98.1")
	notes := func(tag, c string) string {
		return mustRun(t, dir, nil, "sh", script(t, "release-notes.sh"), tag, c, "1.27.2")
	}
	head := commit(t, dir, "feat: first change")
	first := notes("bdock/v0.1.0", head)
	for _, want := range []string{"bdock v0.1.0", head, "Go 1.27.2", "linux/amd64, linux/arm64, darwin/arm64", "bdock_0.1.0_<os>_<arch>.tar.gz", "sha256sum --check --ignore-missing SHA256SUMS", "bdock --version", "prints `bdock version 0.1.0`", "not signed or notarized", "First release."} {
		if !strings.Contains(first, want) {
			t.Errorf("first release notes miss %q:\n%s", want, first)
		}
	}
	if strings.Contains(first, "toolbox/") {
		t.Error("image tags must not become the baseline")
	}
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "release", "bdock/v0.1.0")
	commit(t, dir, "fix: second change")
	head2 := commit(t, dir, "docs: third change")
	second := notes("bdock/v0.1.1", head2)
	if !strings.Contains(second, "Changes since bdock/v0.1.0") || !strings.Contains(second, "- fix: second change") || !strings.Contains(second, "- docs: third change") || strings.Contains(second, "first change") {
		t.Errorf("second release notes wrong:\n%s", second)
	}
}

// fakeRelease builds the archive for one target with the real scripts/release.sh and a
// fake compiler whose "binary" is a shell script, in a throwaway clean repository.
func fakeRelease(t *testing.T, version, target string, mutate func(string)) (archive, commitID, repo string) {
	return fakeReleaseIn(t, version, target, mutate, nil)
}

func fakeReleaseIn(t *testing.T, version, target string, mutate func(string), dirty func(repo string)) (archive, commitID, repo string) {
	t.Helper()
	repo = gitRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"LICENSE", "scripts/release.sh"} {
		raw, err := os.ReadFile(filepath.Join(repoRoot(t), f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, f), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustRun(t, repo, nil, "git", "add", "-A")
	commitID = commit(t, repo, "add release inputs")
	if dirty != nil {
		dirty(repo)
	}
	bin := t.TempDir()
	fakeBdock := `#!/bin/sh
case "$*" in
--version) echo "bdock version @VERSION@" ;;
--help) printf 'Build and manage OCI bundles\n\nAvailable Commands:\n  completion\n  tools\n' ;;
'tools --help') printf 'Manage shared OCI tools\n\nAvailable Commands:\n  upgrade\n  volume\n' ;;
'completion bash') echo '# bash completion for bdock' ;;
*) exit 2 ;;
esac
`
	compiler := `#!/bin/sh
if [ "$1" = version ]; then echo 'go version fixture'; exit 0; fi
out=; ver=
while [ $# -gt 0 ]; do
  case "$1" in -o) out=$2; shift ;; -ldflags) ver=${2#-X main.version=}; shift ;; esac
  shift
done
printf '%s' '` + strings.ReplaceAll(fakeBdock, "'", `'\''`) + `' | sed "s/@VERSION@/$ver/" > "$out"
chmod 0755 "$out"
`
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(compiler), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	mustRun(t, repo, []string{"GO=" + filepath.Join(bin, "go"), "VERSION=" + version, "TARGETS=" + target, "RELEASE_DIR=" + out}, "sh", "scripts/release.sh")
	archive = filepath.Join(out, "bdock_"+version+"_"+strings.ReplaceAll(target, "/", "_")+".tar.gz")
	if mutate != nil {
		mutate(archive)
	}
	sum := mustRun(t, out, nil, "sha256sum", filepath.Base(archive))
	if err := os.WriteFile(archive+".sha256", []byte(sum), 0o644); err != nil {
		t.Fatal(err)
	}
	return archive, commitID, repo
}

func smoke(t *testing.T, repo, version, commitID, target, archive string) (string, error) {
	osName, arch, _ := strings.Cut(target, "/")
	return runIn(t, repo, nil, "sh", script(t, "release-smoke.sh"), version, commitID, osName, arch, archive)
}

func TestSmokeAcceptsTheShippedArchive(t *testing.T) {
	for _, target := range []string{"linux/amd64", "darwin/arm64"} {
		archive, c, repo := fakeRelease(t, "0.1.0", target, nil)
		if out, err := smoke(t, repo, "0.1.0", c, target, archive); err != nil {
			t.Errorf("%s: smoke rejected a good archive: %s", target, out)
		}
	}
}

func TestSmokeRejectsWrongArchives(t *testing.T) {
	archive, c, repo := fakeRelease(t, "0.1.0", "linux/arm64", nil)
	cases := map[string]func() (string, error){
		"wrong version": func() (string, error) { return smoke(t, repo, "0.1.1", c, "linux/arm64", archive) },
		"wrong commit": func() (string, error) {
			return smoke(t, repo, "0.1.0", strings.Repeat("0", 40), "linux/arm64", archive)
		},
		"wrong architecture": func() (string, error) { return smoke(t, repo, "0.1.0", c, "linux/amd64", archive) },
		"wrong os":           func() (string, error) { return smoke(t, repo, "0.1.0", c, "darwin/arm64", archive) },
	}
	for name, run := range cases {
		if out, err := run(); err == nil {
			t.Errorf("%s accepted: %s", name, out)
		}
	}
	// A damaged archive fails its checksum.
	damaged := filepath.Join(filepath.Dir(archive), "bdock_0.1.0_linux_arm64.tar.gz")
	raw, _ := os.ReadFile(damaged)
	if err := os.WriteFile(damaged, append(raw, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := smoke(t, repo, "0.1.0", c, "linux/arm64", archive); err == nil {
		t.Errorf("damaged archive accepted: %s", out)
	}
}

func TestSmokeRejectsMutatedContents(t *testing.T) {
	repack := func(edit func(dir string)) func(string) {
		return func(archive string) {
			dir := t.TempDir()
			mustRun(t, dir, nil, "tar", "-xzf", archive)
			edit(dir)
			files := strings.Fields(mustRun(t, dir, nil, "ls"))
			args := append([]string{"-czf", archive}, files...)
			mustRun(t, dir, nil, "tar", args...)
		}
	}
	mutants := map[string]func(string){
		"extra member":    repack(func(d string) { os.WriteFile(filepath.Join(d, "extra"), []byte("x"), 0o644) }),
		"missing license": repack(func(d string) { os.Remove(filepath.Join(d, "LICENSE")) }),
		"modified source": repack(func(d string) {
			replaceIn(t, filepath.Join(d, "PROVENANCE"), "source_modified=false", "source_modified=true")
		}),
		"foreign license":             repack(func(d string) { os.WriteFile(filepath.Join(d, "LICENSE"), []byte("other"), 0o644) }),
		"not executable":              repack(func(d string) { os.Chmod(filepath.Join(d, "bdock"), 0o644) }),
		"broken help":                 repack(func(d string) { os.WriteFile(filepath.Join(d, "bdock"), []byte("#!/bin/sh\nexit 1\n"), 0o755) }),
		"unstamped version":           repack(func(d string) { replaceIn(t, filepath.Join(d, "bdock"), "bdock version 0.1.0", "bdock version dev") }),
		"empty completion":            repack(func(d string) { replaceIn(t, filepath.Join(d, "bdock"), "echo '# bash completion for bdock'", "true") }),
		"tools help trimmed":          repack(func(d string) { replaceIn(t, filepath.Join(d, "bdock"), "  volume", "  other") }),
		"provenance without CGO line": repack(func(d string) { replaceIn(t, filepath.Join(d, "PROVENANCE"), "CGO_ENABLED=0", "CGO_ENABLED=1") }),
		"provenance without GOOS":     repack(func(d string) { replaceIn(t, filepath.Join(d, "PROVENANCE"), "GOOS=linux", "GOOS=plan9") }),
		"provenance version suffix":   repack(func(d string) { replaceIn(t, filepath.Join(d, "PROVENANCE"), "version=0.1.0", "version=0.1.0-extra") }),
		"provenance revision prefix":  repack(func(d string) { replaceIn(t, filepath.Join(d, "PROVENANCE"), "source_revision=", "source_revision=0") }),
		"wrong command field":         repack(func(d string) { replaceIn(t, filepath.Join(d, "PROVENANCE"), "command=bdock", "command=other") }),
	}
	for name, mutate := range mutants {
		archive, c, repo := fakeRelease(t, "0.1.0", "linux/amd64", mutate)
		if out, err := smoke(t, repo, "0.1.0", c, "linux/amd64", archive); err == nil {
			t.Errorf("mutant %q accepted: %s", name, out)
		}
	}
}

func replaceIn(t *testing.T, path, from, to string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), from) {
		t.Fatalf("HARNESS ERROR, NOT A KILL: %s does not contain %q", path, from)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), from, to, 1)), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseNotesBaselineIsTheNewestStableVersion(t *testing.T) {
	dir := gitRepo(t)
	// Created in non-version order: the newest stable version is not the newest tag, and a
	// pre-release is never a baseline.
	commit(t, dir, "a")
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "r", "bdock/v0.2.0")
	commit(t, dir, "b")
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "r", "bdock/v0.1.9")
	commit(t, dir, "c")
	mustRun(t, dir, nil, "git", "tag", "-a", "-m", "r", "bdock/v0.3.0-rc1")
	head := commit(t, dir, "d")
	out := mustRun(t, dir, nil, "sh", script(t, "release-notes.sh"), "bdock/v0.3.0", head, "1.27.2")
	if !strings.Contains(out, "Changes since bdock/v0.2.0\n") {
		t.Errorf("wrong baseline:\n%s", out)
	}
}

// On macOS there is no sha256sum; the smoke script must fall back to shasum -a 256 -c.
func TestSmokeUsesShasumWhenSha256sumIsAbsent(t *testing.T) {
	archive, c, repo := fakeRelease(t, "0.1.0", "darwin/arm64", nil)
	tools := t.TempDir()
	for _, name := range []string{"sh", "tar", "sort", "tr", "grep", "cmp", "mktemp", "rm", "basename", "dirname", "cat", "sed", "head", "gzip"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	realSum, err := exec.LookPath("sha256sum")
	if err != nil {
		t.Skip("no sha256sum to delegate to")
	}
	log := filepath.Join(t.TempDir(), "calls")
	shim := "#!/bin/sh\necho \"$*\" >> " + log + "\n[ \"$1 $2\" = '-a 256' ] || exit 9\nshift 2\nexec " + realSum + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(tools, "shasum"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", script(t, "release-smoke.sh"), "0.1.0", c, "darwin", "arm64", archive)
	cmd.Dir = repo
	cmd.Env = []string{"PATH=" + tools}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smoke without sha256sum failed: %v\n%s", err, out)
	}
	if raw, _ := os.ReadFile(log); !strings.Contains(string(raw), "-a 256 -c") {
		t.Errorf("shasum -a 256 -c was not used: %q", raw)
	}
}

func provenanceOf(t *testing.T, archive string) string {
	dir := t.TempDir()
	mustRun(t, dir, nil, "tar", "-xzf", archive)
	raw, err := os.ReadFile(filepath.Join(dir, "PROVENANCE"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestProvenanceRecordsRevisionAndDirtyState(t *testing.T) {
	archive, c, _ := fakeRelease(t, "0.1.0", "linux/amd64", nil)
	got := provenanceOf(t, archive)
	if !strings.Contains(got, "source_revision="+c+"\n") || !strings.Contains(got, "source_modified=false\n") {
		t.Errorf("clean build provenance wrong:\n%s", got)
	}
	for name, dirty := range map[string]func(string){
		"untracked file": func(r string) { os.WriteFile(filepath.Join(r, "stray"), []byte("x"), 0o644) },
		"edited file":    func(r string) { os.WriteFile(filepath.Join(r, "LICENSE"), []byte("changed"), 0o644) },
	} {
		archive, c, repo := fakeReleaseIn(t, "0.1.0", "linux/amd64", nil, dirty)
		got := provenanceOf(t, archive)
		if !strings.Contains(got, "source_modified=true\n") {
			t.Errorf("%s: provenance does not record the dirty tree:\n%s", name, got)
		}
		if out, err := smoke(t, repo, "0.1.0", c, "linux/amd64", archive); err == nil {
			t.Errorf("%s: smoke accepted a modified-source archive: %s", name, out)
		}
	}
}

func monotonic(t *testing.T, tag, existing string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", script(t, "release-verify.sh"), "monotonic", tag)
	cmd.Stdin = strings.NewReader(existing)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func TestVerifyMonotonicVersion(t *testing.T) {
	// ls-remote always lists the pushed tag, so every real input contains the tag itself.
	existing := "bdock/v0.1.0\nbdock/v0.2.0\nbdock/v0.10.0\n\nbdock/v0.1.0-rc1\ntoolbox/2026.10.15-b98.1\n"
	ok := map[string]string{
		"bdock/v0.10.1": existing + "bdock/v0.10.1\n", // numeric, not lexical: 0.10.1 > 0.10.0 > 0.2.0
		"bdock/v1.0.0":  existing + "bdock/v1.0.0\nbdock/v1.0.0\n",
		"bdock/v0.1.0":  "bdock/v0.1.0\n", // first release
		"bdock/v0.3.0":  "bdock/v0.3.0\nbdock/v0.3.0\nbdock/v0.2.9\n",
	}
	for tag, ex := range ok {
		if out, err := monotonic(t, tag, ex); err != nil {
			t.Errorf("%s over %q rejected: %s", tag, ex, out)
		}
	}
	// Pre-releases never count as an existing version (its own case: not a map key of ok).
	if out, err := monotonic(t, "bdock/v1.0.0", "bdock/v2.0.0-rc1\nbdock/v1.0.0\n"); err != nil {
		t.Errorf("a higher pre-release blocked 1.0.0: %s", out)
	}
	bad := []struct{ tag, in string }{
		{"bdock/v0.2.0", existing + "bdock/v0.2.0\n"}, // older than 0.10.0
		{"bdock/v0.9.0", existing + "bdock/v0.9.0\n"}, // numeric: 0.9.0 < 0.10.0
		{"bdock/v0.1.5", "bdock/v0.2.0\nbdock/v0.1.5\n"},
		{"bdock/v1.0.0", "bdock/v1.0.1\nbdock/v1.0.0\n"},
		{"bdock/v0.1.0", "bdock/v0.0.1\nbdock/v0.1.0\nv0.1.0\nbdock/v0.1.1\n"},
		// Fail closed when the listing does not show the pushed tag: empty, garbled, CRLF,
		// leading space, or a different tag only.
		{"bdock/v0.1.0", ""},
		{"bdock/v0.1.0", "\n"},
		{"bdock/v0.1.0", "garbage<html>\n"},
		{"bdock/v0.1.0", "bdock/v0.1.0\r\n"},
		{"bdock/v0.1.0", " bdock/v0.1.0\n"},
		{"bdock/v0.5.0", "bdock/v0.4.0\n"},
	}
	for _, c := range bad {
		if out, err := monotonic(t, c.tag, c.in); err == nil {
			t.Errorf("%s over %q accepted: %s", c.tag, c.in, out)
		}
	}
	if _, err := monotonic(t, "bdock/v0.1.0-rc1", "bdock/v0.1.0-rc1\n"); err == nil {
		t.Error("a non-release tag is accepted")
	}
}
