package toolpublish

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"bundledock/internal/toolconfig"
)

// The toolbox Dockerfile carries the stage layout of the compile mechanism. It is
// never built here: these tests only read it.
func toolboxDockerfile(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../bundles/toolbox/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestToolboxDockerfileStagesFollowTheCompileRules(t *testing.T) {
	text := toolboxDockerfile(t)
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

// The toolbox has exactly one compile stage, fetch line and builder copy for each
// source-build tool of its catalog, so a catalog entry cannot ship without its stage
// (and a stage cannot outlive its entry).
func TestToolboxDockerfileHasOneCompileStagePerSourceBuildTool(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	text := toolboxDockerfile(t)
	var tools []string
	for name, p := range c.Catalog.Packages {
		if p.Provider == "source-build" {
			tools = append(tools, name)
		}
	}
	sort.Strings(tools)
	stages := regexp.MustCompile(`(?m)^FROM .* AS compile-([a-z0-9-]+)$`).FindAllStringSubmatch(text, -1)
	var got []string
	for _, m := range stages {
		got = append(got, m[1])
	}
	if strings.Join(got, " ") != strings.Join(tools, " ") {
		t.Fatalf("compile stages %v, source-build tools %v", got, tools)
	}
	for _, tool := range tools {
		for _, want := range []string{
			"RUN tools-bundle fetch-sources -tool " + tool + ` -arch "${TARGETARCH}" -work /build/work/` + tool + "\n",
			"COPY --from=compile-" + tool + " /out /compiled/" + tool + "\n",
			"RUN --network=none tools-bundle compile -tool " + tool + ` -arch "${TARGETARCH}" -work /work -output /out` + "\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the Dockerfile lacks %q", want)
			}
		}
	}
}
