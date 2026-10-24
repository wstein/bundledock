package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildStagesUseMinimalAlpineBases(t *testing.T) {
	data, err := os.ReadFile("../../bundles/toolbox/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(data)
	stages := strings.Split(dockerfile[strings.Index(dockerfile, "FROM "):], "\nFROM ")
	// A reviewed static source build adds one "compile" stage between downloads
	// and builder (AGENTS.md); without one the toolbox has the four prebuilt stages.
	var compile []string
	var rest []string
	for _, stage := range stages {
		if isCompileStage(stage) {
			compile = append(compile, stage)
		} else {
			rest = append(rest, stage)
		}
	}
	stages = rest
	if len(stages) != 4 {
		t.Fatalf("expected compiler, downloads, builder and carrier stages, got %d", len(stages))
	}
	compiler, downloads, builder := stages[0], stages[1], stages[2]
	base := strings.Fields(strings.SplitN(downloads, "\n", 2)[0])
	for _, stage := range compile {
		for _, v := range compileStageViolations(stage, base[len(base)-3]) {
			t.Error(v)
		}
	}
	if strings.Contains(dockerfile, "rust:") || strings.Contains(dockerfile, "git-trim") {
		t.Fatal("the toolbox builds only reviewed static C source recipes, never the removed git-trim Rust recipe")
	}
	for _, stage := range []struct{ name, body, base string }{
		{"compiler", compiler, "FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:"},
		{"downloads", "FROM " + downloads, "FROM --platform=$BUILDPLATFORM alpine:3.23@sha256:"},
	} {
		if !strings.Contains(stage.body, stage.base) {
			t.Errorf("%s must use a digest-pinned build-platform Alpine base", stage.name)
		}
	}
	if !strings.Contains(downloads, "RUN apk add --no-cache ca-certificates\n") {
		t.Error("downloads must install only HTTPS trust certificates without a package cache")
	}
	for _, unwanted := range []string{"apt-get", "debian:", "apk add --no-cache curl", "apk add --no-cache tar", "apk add --no-cache gcc"} {
		if strings.Contains(strings.Join(append([]string{compiler, downloads, builder}, stages[3:]...), "\nFROM "), unwanted) {
			t.Errorf("unnecessary build dependency: %s", unwanted)
		}
	}
	for _, required := range []string{
		"COPY go.mod go.sum ./\nRUN go mod download\n",
		"CGO_ENABLED=0 go build -trimpath -o /out/tools-bundle ./cmd/tools-bundle",
	} {
		if !strings.Contains(compiler, required) {
			t.Errorf("compiler must retain %s", required)
		}
	}
	for _, required := range []string{
		"COPY --from=compiler /out/tools-bundle /usr/local/bin/tools-bundle",
		"COPY bundles/toolbox/packages.txt bundles/toolbox/packages.catalog.json bundles/toolbox/packages.lock.json /build/",
		"COPY bundles/toolbox/licenses /build/licenses",
	} {
		if !strings.Contains(downloads, required) {
			t.Errorf("downloads must retain %s", required)
		}
	}
	if !strings.HasPrefix(builder, "downloads AS builder\nARG TARGETARCH\n") || !strings.Contains(builder, `RUN tools-bundle -arch "${TARGETARCH}"`) {
		t.Error("builder must use the static Go downloader for the target architecture")
	}
}

func TestScratchCarrierContract(t *testing.T) {
	for _, bundle := range []string{"toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu"} {
		t.Run(bundle, func(t *testing.T) {
			data, err := os.ReadFile("../../bundles/" + bundle + "/Dockerfile")
			if err != nil {
				t.Fatal(err)
			}
			dockerfile := string(data)
			final := dockerfile[strings.LastIndex(dockerfile, "\nFROM ")+1:]
			if !strings.HasPrefix(final, "FROM scratch\n") {
				t.Fatal("final carrier must start empty")
			}
			for _, line := range strings.Split(strings.TrimSpace(final), "\n")[1:] {
				if !strings.HasPrefix(line, "COPY ") && !strings.HasPrefix(line, "ENV ") && !strings.HasPrefix(line, "ENTRYPOINT ") {
					t.Fatalf("unexpected final carrier content: %s", line)
				}
			}
			for _, required := range []string{
				"-destination /image-root/opt/bdock/" + bundle,
				"ln -s /opt/bdock /image-root/opt/bundledock",
				"COPY --from=builder /image-root/opt/ /opt/",
				"COPY --from=compiler /out/init-volume /usr/local/bin/init-volume",
				`ENV PATH="/opt/bdock/` + bundle + `/bin:/usr/local/bin"`,
			} {
				if !strings.Contains(dockerfile, required) {
					t.Errorf("missing carrier contract: %s", required)
				}
			}
			if bundle == "toolbox" {
				if !strings.Contains(final, `ENTRYPOINT ["/usr/local/bin/init-volume"]`) {
					t.Error("toolbox must use the initializer default")
				}
			} else if !strings.Contains(final, `"--source", "/opt/bdock/`+bundle+`"`) {
				t.Error("initializer must select its canonical bundle root")
			}
			if strings.Count(final, "COPY ") != 2 || strings.Contains(final, "${PATH}") {
				t.Fatal("carrier must contain only the isolated bundle tree and initializer with explicit PATH")
			}
			if !strings.Contains(dockerfile, "CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/init-volume") {
				t.Fatal("initializer must be static and target-native")
			}
		})
	}
}

func TestSharedVolumeSmokeWithShellFreeCarrier(t *testing.T) {
	dir := t.TempDir()
	docker := `#!/bin/sh
case "$1" in
image) printf '%s\n' sha256:carrier ;;
volume)
 if [ "$2" = create ]; then printf '%s\n' fixture-volume; fi ;;
run)
 shift
 entrypoint=
 mount=
 mounts=
 while [ "$#" -gt 0 ]; do
  case "$1" in
   --rm) shift ;;
   --entrypoint) entrypoint=$2; shift 2 ;;
   --mount)
    case "$2" in
     type=bind,src=*) source=${2#type=bind,src=}; source=${source%%,dst=*}; [ -f "$source" ] || exit 67 ;;
    esac
    mount=$2; mounts="$mounts $2"; shift 2 ;;
   --network|--user|--tmpfs|--env) shift 2 ;;
   *) image=$1; shift; break ;;
  esac
 done
 case "$image" in
  sha256:carrier)
   [ -z "$entrypoint" ] || { echo 'scratch has no shell' >&2; exit 61; }
   if [ -f "$CARRIER_STATE" ]; then
    printf 'rejected %s\n' "$(cat "$CARRIER_STATE")" >> "$CARRIER_CALLS"
    exit 1
   fi
   printf '%s\n' initialized >> "$CARRIER_CALLS" ;;
  consumer:fixture)
   [ "$entrypoint" = /bin/sh ] || exit 62
   case "$mount" in
    *dst=/bundle,*)
     case "$*" in
      *manifest.json*) state=mismatched ;;
      *'rm /bundle/.ready'*) state=incomplete ;;
      *'mkdir /bundle/.initializing'*) state=locked ;;
      *) exit 63 ;;
     esac
     printf '%s\n' "$state" > "$CARRIER_STATE"
     printf 'mutated %s\n' "$state" >> "$CARRIER_CALLS" ;;
    *)
     case "$mounts" in
      *dst=/opt/bdock/toolbox,readonly,volume-nocopy*) ;;
      *) echo 'consumer must mount the named toolbox root read-only' >&2; exit 66 ;;
     esac
     printf '%s\n' consumer >> "$CARRIER_CALLS" ;;
   esac ;;
  *) exit 64 ;;
 esac ;;
*) exit 65 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(docker), 0700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(dir, "calls")
	cmd := exec.Command("sh", "../../bundles/toolbox/scripts/smoke.sh", "carrier:fixture", "consumer:fixture")
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "CARRIER_CALLS="+calls, "CARRIER_STATE="+filepath.Join(dir, "state"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shell-free carrier smoke: %v\n%s", err, output)
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Count(got, "initialized\n") != 2 || strings.Count(got, "consumer\n") != 2 {
		t.Fatalf("missing repeat initialization or concurrent consumers: %s", got)
	}
	for _, state := range []string{"mismatched", "incomplete", "locked"} {
		if !strings.Contains(got, "mutated "+state+"\nrejected "+state+"\n") {
			t.Fatalf("missing consumer mutation/carrier rejection for %s: %s", state, got)
		}
	}
}

func isCompileStage(stage string) bool {
	f := strings.Fields(strings.SplitN(stage, "\n", 2)[0])
	return len(f) >= 3 && strings.EqualFold(f[len(f)-2], "AS") && strings.HasPrefix(f[len(f)-1], "compile-")
}

// dockerfileInstructions joins continuation lines and returns the instruction
// keyword (upper case) and arguments of every instruction after the FROM line.
func dockerfileInstructions(stage string) [][2]string {
	var out [][2]string
	joined := strings.ReplaceAll(stage, "\\\n", " ")
	for _, line := range strings.Split(joined, "\n")[1:] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kw, args, _ := strings.Cut(line, " ")
		out = append(out, [2]string{strings.ToUpper(kw), strings.TrimSpace(args)})
	}
	return out
}

// compileStageViolations checks the conditions on a source-build stage
// "compile-<tool>": the same digest-pinned base as the downloads stage (base),
// no package installation, no network, and inputs only from the verified fetch
// stage (COPY --from=downloads); no ADD, no COPY from the build context, no
// downloader or VCS tool, no mounts.
func compileStageViolations(stage, base string) []string {
	var out []string
	first := strings.Fields(strings.SplitN(stage, "\n", 2)[0])
	if len(first) < 3 || first[len(first)-3] != base || !strings.Contains(base, "@sha256:") {
		out = append(out, "compile stage must use the digest-pinned base of the downloads stage")
	}
	runs := 0
	for _, in := range dockerfileInstructions(stage) {
		kw, args := in[0], in[1]
		switch kw {
		case "ARG", "ENV", "WORKDIR":
		case "COPY":
			if !strings.HasPrefix(args, "--from=downloads ") {
				out = append(out, "compile stage COPY must be --from=downloads: "+args)
			}
		case "RUN":
			runs++
			if !runFlag(args, "--network=none") {
				out = append(out, "compile stage RUN must use --network=none: "+args)
			}
			if strings.Contains(args, "--mount") {
				out = append(out, "compile stage RUN must not use mounts: "+args)
			}
			for _, tool := range []string{"apk", "apt", "apt-get", "curl", "wget", "git", "pip", "npm", "go", "cargo", "nc"} {
				for _, w := range strings.FieldsFunc(args, func(r rune) bool { return strings.ContainsRune(" \t;&|()<>=\"'`$", r) }) {
					if w == tool || strings.HasSuffix(w, "/"+tool) {
						out = append(out, "compile stage RUN uses "+tool+": "+args)
					}
				}
			}
		default:
			out = append(out, "compile stage instruction not allowed: "+kw+" "+args)
		}
	}
	if runs == 0 {
		out = append(out, "compile stage has no RUN")
	}
	return out
}

func TestCompileStageViolations(t *testing.T) {
	const base = "alpine:3.23@sha256:abc"
	head := "FROM --platform=$TARGETPLATFORM " + base + " AS compile-sqlite3\n"
	good := head + "COPY --from=downloads /build/src /src\nWORKDIR /src\nRUN --network=none zig cc -static -o /out/sqlite3 shell.c\n"
	if v := compileStageViolations(good, base); len(v) != 0 {
		t.Fatalf("conforming stage rejected: %v", v)
	}
	loose := head + "  copy --from=downloads /build/src /src\n\t# comment\n  run --network=none zig cc \\\n    -static shell.c\n"
	if v := compileStageViolations(loose, base); len(v) != 0 {
		t.Fatalf("lowercase or indented instructions rejected: %v", v)
	}
	if !isCompileStage(good) || isCompileStage("FROM x AS compiler\n") || isCompileStage("FROM x AS compile\n") {
		t.Error("stage name must be compile-<tool>")
	}
	for name, bad := range map[string]string{
		"unpinned base":              "FROM alpine:3.23 AS compile-x\nRUN --network=none make\n",
		"other pinned base":          "FROM debian:12@sha256:abc AS compile-x\nRUN --network=none make\n",
		"network":                    head + "RUN make\n",
		"network flag late":          head + "RUN make --network=none\n",
		"network flag after command": head + "RUN make --network=none all\n",
		"network in sh -c":           head + "RUN sh -c 'make --network=none'\n",
		"network flag value":         head + "RUN --network=host --x=--network=none make\n",
		"network none then host":     head + "RUN --network=none --network=host make\n",
		"network host then none":     head + "RUN --network=host --network=none make\n",
		"network none twice":         head + "RUN --network=none --network=none make\n",
		"network none then default":  head + "RUN --network=none --network=default make\n",
		"network space form":         head + "RUN --network none make\n",
		"network space form host":    head + "RUN --network=none --network host make\n",
		"network default":            head + "RUN --network=default make\n",
		"lowercase run":              head + "run make\n",
		"indented run":               head + "  RUN make\n",
		"apk add":                    head + "RUN --network=none apk add gcc\n",
		"curl":                       head + "RUN --network=none curl -O https://x/y\n",
		"wget path":                  head + "RUN --network=none /usr/bin/wget https://x/y\n",
		"git clone":                  head + "RUN --network=none git clone https://x/y\n",
		"chained":                    head + "RUN --network=none make && curl x\n",
		"continued":                  head + "RUN --network=none make \\\n && wget x\n",
		"add url":                    head + "ADD https://x/y /src/y\nRUN --network=none make\n",
		"lowercase add":              head + "add https://x/y /src/y\nRUN --network=none make\n",
		"copy context":               head + "COPY src /src\nRUN --network=none make\n",
		"copy other stage":           head + "COPY --from=compiler /out /src\nRUN --network=none make\n",
		"copy builder":               head + "COPY --from=builder /out /src\nRUN --network=none make\n",
		"mount":                      head + "RUN --network=none --mount=type=bind,source=.,target=/src make\n",
		"no run":                     head + "COPY --from=downloads /a /b\n",
		"onbuild":                    head + "ONBUILD RUN make\nRUN --network=none make\n",
	} {
		if len(compileStageViolations(bad, base)) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

// runFlag reports whether the leading --flags of a RUN instruction (before
// the command; text after the command never counts) contain flag exactly once
// and no other --network flag, in either "--network=x" or "--network x" form.
func runFlag(args, flag string) bool {
	words := strings.Fields(args)
	found := 0
	for _, w := range words {
		if !strings.HasPrefix(w, "--") {
			break
		}
		if w == flag {
			found++
		} else if w == "--network" || strings.HasPrefix(w, "--network=") {
			return false
		}
	}
	return found == 1
}
