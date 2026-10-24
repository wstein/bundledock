package toolpublish

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The source-build Dockerfile fixture (cmd/tools-bundle/testdata) shows the stage
// layout the compile mechanism expects. It is never built here: these tests only
// read it.
func sourceBuildFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../cmd/tools-bundle/testdata/source-build.Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSourceBuildFixtureStagesFollowTheCompileRules(t *testing.T) {
	text := sourceBuildFixture(t)
	stages := strings.Split(text[strings.Index(text, "FROM "):], "\nFROM ")
	var names []string
	var base string
	for _, stage := range stages {
		stage = "FROM " + strings.TrimPrefix(stage, "FROM ")
		f := strings.Fields(strings.SplitN(stage, "\n", 2)[0])
		if f[len(f)-1] == "downloads" {
			base = f[len(f)-3]
		}
		names = append(names, f[len(f)-1])
	}
	if got := strings.Join(names, " "); got != "compiler downloads compile-sqlite3 builder scratch" {
		t.Fatalf("stage order %q", got)
	}
	compiled := 0
	for _, stage := range stages {
		stage = "FROM " + strings.TrimPrefix(stage, "FROM ")
		if !isCompileStage(stage) {
			continue
		}
		compiled++
		for _, v := range compileStageViolations(stage, base) {
			t.Error(v)
		}
		if !strings.Contains(stage, "--platform=$TARGETPLATFORM") {
			t.Error("the compile stage must run on the target platform (native on the publication runners)")
		}
	}
	if compiled != 1 {
		t.Fatalf("expected one compile stage, got %d", compiled)
	}
	downloads := stages[1]
	if !strings.Contains(downloads, "RUN tools-bundle fetch-sources ") || strings.Contains(downloads, "--network=none") {
		t.Error("sources and the compiler are fetched and verified in the downloads stage, which has the network")
	}
	if !strings.Contains(text, "COPY --from=compile-sqlite3 /out /compiled/sqlite3\n") || !strings.Contains(text, "--build-outputs /compiled") {
		t.Error("the builder must take the compile output and pass it to tools-bundle")
	}
}

// Apart from the source-build additions the fixture is the production toolbox
// Dockerfile, so it cannot drift from it unnoticed.
func TestSourceBuildFixtureDiffersFromProductionOnlyByTheCompileStage(t *testing.T) {
	prod, err := os.ReadFile("../../bundles/toolbox/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	text := sourceBuildFixture(t)
	text = regexp.MustCompile(`(?s)# FIXTURE.*?with the catalog entry\.\n`).ReplaceAllString(text, "")
	start := strings.Index(text, "RUN tools-bundle fetch-sources")
	end := strings.Index(text, "FROM downloads AS builder")
	if start < 0 || end < start {
		t.Fatal("fixture lacks the fetch or compile stage")
	}
	text = text[:start] + text[end:]
	for _, add := range []string{
		"COPY --from=compiler /out/tools-bundle-native /build/native/tools-bundle\n",
		"COPY --from=compile-sqlite3 /out /compiled/sqlite3\n",
		" --build-outputs /compiled",
	} {
		if !strings.Contains(text, add) {
			t.Fatalf("fixture lacks %q", add)
		}
		text = strings.Replace(text, add, "", 1)
	}
	if text != string(prod) {
		t.Fatalf("the fixture differs from bundles/toolbox/Dockerfile beyond the source-build lines:\n%s", text)
	}
}
