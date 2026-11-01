package toolpublish

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestBuildArgsPinSourceDateEpoch(t *testing.T) {
	for _, bundle := range []string{"toolbox", "nixbox"} {
		b, err := SelectBundle(bundle)
		if err != nil {
			t.Fatal(err)
		}
		got := buildArgs(b, Metadata{Tag: "2026.11.01-b1"}, "amd64")
		want := []string{"docker", "build", "--pull", "--platform", "linux/amd64",
			"-f", "bundles/" + bundle + "/Dockerfile", "-t", "bundledock-" + bundle + ":build-2026.11.01-b1-amd64",
			"--build-arg", "SOURCE_DATE_EPOCH=" + SourceDateEpoch}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: args %q, want %q", bundle, got, want)
		}
		if got[buildTagIndex] != want[buildTagIndex] {
			t.Errorf("%s: buildTagIndex %d does not point at the tag", bundle, buildTagIndex)
		}
	}
}

// The epoch must not depend on the version: a per-version time would give every
// version different layer digests, including for unchanged tools.
func TestSourceDateEpochIsConstant(t *testing.T) {
	b, _ := SelectBundle("toolbox")
	a := buildArgs(b, Metadata{Tag: "2026.11.01-b1"}, "amd64")
	c := buildArgs(b, Metadata{Tag: "2026.12.24-b3"}, "amd64")
	if a[10] != c[10] {
		t.Fatalf("epoch differs between versions: %q vs %q", a[6], c[6])
	}
}

var normalizeRoot = regexp.MustCompile(`normalize-mtimes -root (\S+)`)

// Every directory that the final stage copies must have passed through
// normalize-mtimes; a COPY of an un-normalized path would put build-time mtimes
// into a layer. The Dockerfile default must equal the publisher's constant.
func TestDockerfilesNormalizeEveryCopiedRoot(t *testing.T) {
	for _, bundle := range []string{"toolbox", "nixbox"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "bundles", bundle, "Dockerfile"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "\nARG SOURCE_DATE_EPOCH="+SourceDateEpoch+"\n") {
			t.Errorf("%s: Dockerfile lacks ARG SOURCE_DATE_EPOCH=%s", bundle, SourceDateEpoch)
		}
		normalized := map[string]bool{}
		for _, m := range normalizeRoot.FindAllStringSubmatch(text, -1) {
			normalized[strings.TrimRight(m[1], ";")] = true
		}
		_, final, ok := strings.Cut(text, "\nFROM scratch\n")
		if !ok {
			t.Fatalf("%s: no final scratch stage", bundle)
		}
		copies := 0
		for _, line := range strings.Split(final, "\n") {
			f := strings.Fields(line)
			if len(f) == 0 || f[0] != "COPY" {
				continue
			}
			copies++
			if len(f) != 4 || !strings.HasPrefix(f[1], "--from=") || !strings.HasSuffix(f[2], "/") || f[3] != "/" {
				t.Errorf("%s: final COPY must copy a whole staged root to /: %q", bundle, line)
				continue
			}
			if root := strings.TrimSuffix(f[2], "/"); !normalized[root] {
				t.Errorf("%s: %s is copied into a layer but never passed to normalize-mtimes", bundle, root)
			}
		}
		if copies == 0 {
			t.Errorf("%s: final stage has no COPY", bundle)
		}
	}
}
