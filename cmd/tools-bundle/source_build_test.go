package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// The toolbox Dockerfile runs the compile mechanism: the command lines it contains
// must parse with the real subcommand flags.
func TestToolboxDockerfileCommandsParse(t *testing.T) {
	data, err := os.ReadFile("../../bundles/toolbox/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]sourceBuildOptions{}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\\\n", " "), "\n") {
		for _, command := range []string{"fetch-sources", "compile"} {
			_, args, ok := strings.Cut(line, "tools-bundle "+command+" ")
			if !ok {
				continue
			}
			o, err := parseSourceBuild(command, strings.Fields(strings.ReplaceAll(args, `"${TARGETARCH}"`, "arm64")), io.Discard)
			if err != nil {
				t.Fatalf("%s: %v", line, err)
			}
			seen[command] = o
		}
	}
	if seen["fetch-sources"].tool != "sqlite3" || seen["fetch-sources"].arch != "arm64" || seen["fetch-sources"].work != "/build/work/sqlite3" {
		t.Fatalf("fetch-sources: %+v", seen["fetch-sources"])
	}
	c := seen["compile"]
	if c.tool != "sqlite3" || c.work != "/work" || c.output != "/out" || c.directory != "/build" {
		t.Fatalf("compile: %+v", c)
	}
}

func TestSourceBuildArguments(t *testing.T) {
	for name, tc := range map[string]struct {
		command string
		args    []string
		want    string
	}{
		"unknown architecture": {"compile", []string{"-tool", "x", "-arch", "riscv64", "-work", "/w", "-output", "/o"}, "architecture"},
		"no architecture":      {"fetch-sources", []string{"-tool", "x", "-work", "/w"}, "architecture"},
		"no tool":              {"fetch-sources", []string{"-arch", "amd64", "-work", "/w"}, "-tool"},
		"no work":              {"compile", []string{"-tool", "x", "-arch", "amd64", "-output", "/o"}, "-work"},
		"no output":            {"compile", []string{"-tool", "x", "-arch", "amd64", "-work", "/w"}, "-output"},
		"stray argument":       {"compile", []string{"-tool", "x", "-arch", "amd64", "-work", "/w", "-output", "/o", "extra"}, "unexpected"},
		"output on fetch":      {"fetch-sources", []string{"-tool", "x", "-arch", "amd64", "-work", "/w", "-output", "/o"}, "output"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseSourceBuild(tc.command, tc.args, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
		})
	}
}

func TestSourceBuildRejectsAToolThatIsNotASourceBuild(t *testing.T) {
	// gh is an official prebuilt release: both subcommands must refuse it before any download.
	for _, command := range []string{"fetch-sources", "compile"} {
		args := []string{"-tool", "gh", "-arch", "amd64", "-directory", "../../bundles/toolbox", "-work", t.TempDir() + "/w", "-output", t.TempDir()}
		if command == "fetch-sources" {
			args = args[:len(args)-2]
		}
		if err := runSourceBuild(command, args); err == nil || !strings.Contains(err.Error(), "not a source-build tool") {
			t.Fatalf("%s: %v", command, err)
		}
	}
}
