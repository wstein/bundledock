package toolpublish

import (
	"bundledock/internal/toolconfig"
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
	if len(stages) != 5 {
		t.Fatalf("expected compiler, git-trim, downloads, builder and carrier stages, got %d", len(stages))
	}
	compiler, gitTrim, downloads, builder := stages[0], stages[1], stages[2], stages[3]
	if !strings.HasPrefix(gitTrim, "--platform=$TARGETPLATFORM "+toolconfig.GitTrimBuilder+" AS git-trim-source\n") {
		t.Fatal("source exception must use the fixed stock native Rust image")
	}
	if !strings.Contains(gitTrim, "RUN --network=none sh /build/build-git-trim.sh compile") {
		t.Fatal("source compilation must be offline")
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
		if strings.Contains(dockerfile, unwanted) {
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
	data, err := os.ReadFile("../../bundles/toolbox/Dockerfile")
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
		"COPY --from=builder /opt/bundledock/toolbox /opt/bundledock/toolbox",
		"COPY --from=compiler /out/init-volume /usr/local/bin/init-volume",
		`ENTRYPOINT ["/usr/local/bin/init-volume"]`,
		`ENV PATH="/opt/bundledock/toolbox/bin:/usr/local/bin"`,
	} {
		if !strings.Contains(final, required) {
			t.Fatalf("missing carrier contract: %s", required)
		}
	}
	if strings.Count(final, "COPY ") != 2 || strings.Contains(final, "${PATH}") {
		t.Fatal("carrier must contain only bundle and initializer with explicit PATH")
	}
	if !strings.Contains(dockerfile, "CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/init-volume") {
		t.Fatal("initializer must be static and target-native")
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
      *dst=/opt/bundledock/toolbox,readonly,volume-nocopy*) ;;
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
