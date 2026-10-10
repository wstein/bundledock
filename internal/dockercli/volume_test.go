package dockercli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestImmutableIDAndForwardedOutput(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	volume := "bundledock-toolbox-" + strings.Repeat("a", 64)
	image := "arbitrary image; $(touch ignored)"
	var calls [][]string
	var out, stderr bytes.Buffer
	runner := func(ctx context.Context, stdout, errout io.Writer, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			fmt.Fprintln(stdout, id)
		}
		if args[0] == "run" {
			fmt.Fprintln(stdout, "Tool volume initialized")
		}
		return nil
	}
	if err := Volume(context.Background(), runner, image, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{{"image", "inspect", "--format", "{{.Id}}", "--", image}, {"volume", "create", volume}, {"run", "--rm", "--network", "none", "--mount", "type=volume,src=" + volume + ",dst=/bundle,volume-nocopy", id}}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("args %#v", calls)
	}
	if out.String() != "Tool volume initialized\nVolume: "+volume+"\n" {
		t.Fatalf("output %q", out.String())
	}
}
func TestFailuresStopDockerSequence(t *testing.T) {
	for _, stage := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(stage), func(t *testing.T) {
			calls := 0
			cause := errors.New("docker failed")
			var out bytes.Buffer
			runner := func(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
				calls++
				if calls == stage {
					return cause
				}
				if calls == 1 {
					fmt.Fprintln(stdout, "sha256:"+strings.Repeat("1", 64))
				}
				return nil
			}
			if err := Volume(context.Background(), runner, "tools", &out, io.Discard); !errors.Is(err, cause) {
				t.Fatalf("error %v", err)
			}
			if calls != stage || strings.Contains(out.String(), "Volume:") {
				t.Fatalf("continued after failure calls=%d output=%q", calls, out.String())
			}
		})
	}
	for _, id := range []string{"", "sha256:short", "sha256:" + strings.Repeat("g", 64), "sha256:" + strings.Repeat("a", 64) + "\nsecond"} {
		calls := 0
		runner := func(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
			calls++
			fmt.Fprintln(stdout, id)
			return nil
		}
		if err := Volume(context.Background(), runner, "tools", io.Discard, io.Discard); err == nil || calls != 1 {
			t.Fatalf("accepted image ID %q", id)
		}
	}
}
func fakeDocker(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("requires sh: ", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
func TestProcessArgumentsAndFailure(t *testing.T) {
	fakeDocker(t, "printf '%s\\n' \"$@\"\nprintf 'diagnostic' >&2\nexit 7\n")
	var out, stderr bytes.Buffer
	err := Run(context.Background(), &out, &stderr, "one argument", "$(literal)")
	if err == nil || out.String() != "one argument\n$(literal)\n" || stderr.String() != "diagnostic" {
		t.Fatalf("err=%v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
}
func TestProcessCancellation(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("requires sleep: ", err)
	}
	fakeDocker(t, "exec sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := Run(ctx, io.Discard, io.Discard, "run"); err == nil {
		t.Fatal("cancellation succeeded")
	}
	if ctx.Err() != context.DeadlineExceeded || time.Since(start) > 3*time.Second {
		t.Fatalf("cancellation delayed: %v", ctx.Err())
	}
}
func TestCanceledContextPropagatesToRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := func(got context.Context, _, _ io.Writer, _ ...string) error {
		if got != ctx {
			t.Fatal("context changed")
		}
		return got.Err()
	}
	if err := Volume(ctx, runner, "tools", io.Discard, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDockerOnlyCompatibilityAdapter(t *testing.T) {
	id := "sha256:" + strings.Repeat("b", 64)
	fakeDocker(t, "case \"$1\" in\nimage) printf '%s\\n' '"+id+"' ;;\nvolume) printf 'create output is discarded\\n' ;;\nrun) printf 'Tool volume initialized\\n' ;;\nesac\n")
	command := exec.Command("sh", "../../bundles/toolbox/volume.sh", "local:fixture")
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("adapter failed: %v %s", err, data)
	}
	want := "Tool volume initialized\nVolume: bundledock-toolbox-" + strings.Repeat("b", 64) + "\n"
	if string(data) != want {
		t.Fatalf("output %q", data)
	}
}

func TestDockerAdapterExplicitLegacyVolume(t *testing.T) {
	id := "sha256:" + strings.Repeat("b", 64)
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("DOCKER_CALLS", log)
	fakeDocker(t, "printf '%s\\n' \"$*\" >> \"$DOCKER_CALLS\"\ncase \"$1\" in\nimage) printf '%s\\n' '"+id+"' ;;\nesac\n")
	legacy := "anchorage-tools-" + strings.Repeat("b", 64)
	output, err := exec.Command("sh", "../../bundles/toolbox/volume.sh", id, legacy).CombinedOutput()
	if err != nil {
		t.Fatalf("adapter: %v %s", err, output)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "volume create "+legacy+"\n") || !strings.Contains(string(calls), "src="+legacy+",dst=/bundle,volume-nocopy "+id) || !strings.Contains(string(output), "Volume: "+legacy) {
		t.Fatalf("calls %s output %s", calls, output)
	}
}

func TestDockerAdapterRejectsInvalidVolumeNames(t *testing.T) {
	id := "sha256:" + strings.Repeat("b", 64)
	fakeDocker(t, "case \"$1\" in\nimage) printf '%s\\n' '"+id+"' ;;\n*) printf 'unexpected mutation\\n' ;;\nesac\n")
	for _, name := range []string{"-option", "has space", "a,b", "a/b", "a\nother"} {
		output, err := exec.Command("sh", "../../bundles/toolbox/volume.sh", id, name).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "Invalid Docker volume name") || strings.Contains(string(output), "unexpected mutation") {
			t.Fatalf("accepted %q: %v %s", name, err, output)
		}
	}
}
