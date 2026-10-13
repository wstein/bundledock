package toolbundle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runQualityUtilitiesSmoke(t *testing.T, broken string, override map[string]string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("../../bundles/toolbox/scripts/quality-utilities-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	stubs := map[string]string{
		"shellcheck": `case "$*" in --version) echo ShellCheck;; *) cat >/dev/null;; esac`,
		"shfmt":      `case "$*" in --version) echo v3.14.1;; *) cat >/dev/null; echo 'echo smoke';; esac`,
		"just":       `case "$*" in --version) echo 'just 1.58.0';; *--quiet*) :;; *) echo recipe-smoke;; esac`,
		"ruff":       `case "$*" in --version) echo 'ruff 0.17.0';; *) cat >/dev/null; echo 'x = 1';; esac`,
		"delta":      `case "$*" in --version) echo 'delta 0.20.1';; '--no-gitconfig --paging=never') cat >/dev/null; printf '\033[48;5;124mbefore\033[48;5;52m-smoke\033[0m\n\033[48;5;28;38;5;231mafter\033[48;5;22m-smoke\033[0m\033[0K\n';; *) exit 2;; esac`,
		"watchexec":  `case "$*" in --version) echo 'watchexec 2.8.0';; '-1 --shell=none --workdir '*) echo watch-smoke;; *) exit 2;; esac`,
		"hyperfine":  `case "$*" in --version) echo 'hyperfine 2.0.0';; *) while test "$1" != --export-json; do shift; done; shift; echo '{"schema_version":2,"results":[{"command":"true","measurements":[{"time_wall_clock":{"value":0.001,"unit":"second"},"exit_code":0}]}]}' > "$1";; esac`,
		// The source suite requires Python, but not an installed toolbox.
		// Model JSON semantics independently; the approved native probe checks
		// the actual jq expression against Hyperfine's official export.
		"jq": `python3 - "$3" <<'PYJSON'
import json, sys
try:
    data = json.load(open(sys.argv[1]))
    results = data["results"]
    measurements = results[0]["measurements"]
    wall = measurements[0]["time_wall_clock"]
    valid = (data["schema_version"] == 2 and len(results) == 1
             and results[0]["command"] == "true" and len(measurements) == 1
             and measurements[0]["exit_code"] == 0 and wall["unit"] == "second"
             and type(wall["value"]) in (int, float) and wall["value"] >= 0)
except (ValueError, KeyError, IndexError, TypeError):
    valid = False
sys.exit(0 if valid else 1)
PYJSON`,
		"age-keygen": `case "$*" in --version) echo v1.3.2;; -o*) echo AGE-SECRET-KEY-SYNTHETIC > "$2";; -y*) echo age1synthetic;; esac`,
		"age":        `case "$*" in --version) echo v1.3.2;; *) while test "$1" != -o; do shift; done; shift; output=$1; shift; cat "$1" > "$output";; esac`,
	}
	home := t.TempDir()
	bin := t.TempDir()
	for name, body := range stubs {
		if name == broken {
			body = "echo wrong"
		}
		if replacement, ok := override[name]; ok {
			body = replacement
		}
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh", script)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestQualityUtilitiesSmokeRejectsWrongBehavior(t *testing.T) {
	for _, broken := range []string{"", "shellcheck", "shfmt", "just", "ruff", "delta", "watchexec", "hyperfine", "age-keygen", "age"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			out, err := runQualityUtilitiesSmoke(t, broken, nil)
			if broken == "" && err != nil {
				t.Fatalf("valid synthetic tools failed: %v\n%s", err, out)
			}
			if broken != "" && err == nil {
				t.Fatalf("broken %s passed", broken)
			}
		})
	}
}

func TestQualityUtilitiesSmokeReportsFailedCommands(t *testing.T) {
	// Even expected stdout must not hide a failed command substitution. These
	// are synthetic control tools, not validation of the official executables.
	for _, tc := range []struct{ tool, stage, output string }{
		{"shellcheck", "shellcheck analysis", ""},
		{"shfmt", "shfmt formatting", "echo smoke"},
		{"just", "just recipe", "recipe-smoke"},
		{"ruff", "ruff formatting", "x = 1"},
		{"delta", "delta rendering", "after-smoke"},
		{"watchexec", "watchexec command", "watch-smoke"},
		{"hyperfine", "hyperfine benchmark", ""},
		{"age-keygen", "age key generation", ""},
		{"age", "age encryption", ""},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			body := fmt.Sprintf("case \"$*\" in --version) echo version;; *) printf '%%s\\n' '%s'; exit 23;; esac", tc.output)
			out, err := runQualityUtilitiesSmoke(t, "", map[string]string{tc.tool: body})
			if err == nil {
				t.Fatalf("failed %s command with expected stdout passed\n%s", tc.tool, out)
			}
			want := "Quality utilities smoke failed: " + tc.stage + " (exit 23)"
			if !strings.Contains(out, want) {
				t.Fatalf("missing diagnostic %q\n%s", want, out)
			}
		})
	}
}

func TestQualityUtilitiesSmokeReportsAssertionWithoutValues(t *testing.T) {
	const sensitive = "AGE-SECRET-KEY-SYNTHETIC-DO-NOT-LOG"
	out, err := runQualityUtilitiesSmoke(t, "", map[string]string{
		"age-keygen": `case "$*" in --version) echo version;; -o*) echo ` + sensitive + ` > "$2";; -y*) echo ` + sensitive + `;; esac`,
	})
	if err == nil {
		t.Fatal("invalid recipient passed")
	}
	if !strings.Contains(out, "Quality utilities smoke failed: age recipient assertion (exit 1)") {
		t.Fatalf("missing recipient assertion diagnostic\n%s", out)
	}
	if strings.Contains(out, sensitive) {
		t.Fatal("diagnostics disclosed a captured value")
	}
}

func TestQualityUtilitiesSmokeDeltaStatusAndRendering(t *testing.T) {
	for _, tc := range []struct{ name, body, diagnostic string }{
		{"ANSI intraline rendering", `cat >/dev/null; printf '\033[48;5;124mbefore\033[48;5;52m-smoke\033[0m\n\033[48;5;28;38;5;231mafter\033[48;5;22m-smoke\033[0m\033[0K\n'`, ""},
		{"unrendered diff", `cat`, "delta removed text assertion"},
		{"missing added text", `cat >/dev/null; echo before-smoke`, "delta added text assertion"},
		{"missing removed text", `cat >/dev/null; echo after-smoke`, "delta removed text assertion"},
		{"diff status one", `cat >/dev/null; printf 'before-smoke\nafter-smoke\n'; exit 1`, "delta rendering"},
		{"error status two", `cat >/dev/null; printf 'before-smoke\nafter-smoke\n'; exit 2`, "delta rendering"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `case "$*" in --version) echo 'delta 0.20.1';; *) ` + tc.body + `;; esac`
			out, err := runQualityUtilitiesSmoke(t, "", map[string]string{"delta": body})
			if tc.diagnostic == "" {
				if err != nil {
					t.Fatalf("valid ANSI rendering failed: %v\n%s", err, out)
				}
				return
			}
			if err == nil || !strings.Contains(out, "Quality utilities smoke failed: "+tc.diagnostic+" (exit ") {
				t.Fatalf("invalid rendering/status accepted or wrong diagnostic: %v\n%s", err, out)
			}
		})
	}
}

func TestQualityUtilitiesSmokeVersionDiagnostics(t *testing.T) {
	for _, tool := range []string{"shellcheck", "shfmt", "just", "ruff", "delta", "watchexec", "hyperfine", "age", "age-keygen"} {
		t.Run(tool, func(t *testing.T) {
			out, err := runQualityUtilitiesSmoke(t, "", map[string]string{tool: `exit 23`})
			if err == nil || !strings.Contains(out, "Quality utilities smoke failed: "+tool+" version (exit 23)") {
				t.Fatalf("missing failed version diagnostic: %v\n%s", err, out)
			}
		})
	}
}

func TestQualityUtilitiesSmokeRecipientStatus(t *testing.T) {
	out, err := runQualityUtilitiesSmoke(t, "", map[string]string{
		"age-keygen": `case "$*" in --version) echo version;; -o*) echo AGE-SECRET-KEY-SYNTHETIC > "$2";; -y*) echo age1synthetic; exit 23;; esac`,
	})
	if err == nil || !strings.Contains(out, "Quality utilities smoke failed: age recipient derivation (exit 23)") {
		t.Fatalf("expected recipient hid command failure: %v\n%s", err, out)
	}
}

func TestQualityUtilitiesSmokeQuietShellcheckFailure(t *testing.T) {
	out, err := runQualityUtilitiesSmoke(t, "", map[string]string{"shellcheck": `case "$*" in --version) echo version;; *) cat >/dev/null; exit 2;; esac`})
	if err == nil || !strings.Contains(out, "Quality utilities smoke failed: shellcheck analysis (exit 2)") {
		t.Fatalf("quiet analysis failure passed or lost status: %v\n%s", err, out)
	}
}

func TestQualityUtilitiesSmokeDecryptionFailure(t *testing.T) {
	out, err := runQualityUtilitiesSmoke(t, "", map[string]string{"age": `case "$*" in --version) echo version;; -d*) exit 23;; *) while test "$1" != -o; do shift; done; shift; output=$1; shift; cat "$1" > "$output";; esac`})
	if err == nil || !strings.Contains(out, "Quality utilities smoke failed: age decryption (exit 23)") {
		t.Fatalf("decryption failure passed or lost status: %v\n%s", err, out)
	}
}

func TestQualityUtilitiesSmokeHyperfineJSON(t *testing.T) {
	for _, data := range []string{
		`not JSON`,
		`{"results":[{"command":"true","times":[0.001]}]}`,
		`{"schema_version":2,"results":[{"command":"false","measurements":[{"time_wall_clock":{"value":0.001,"unit":"second"},"exit_code":0}]}]}`,
		`{"schema_version":2,"results":[{"command":"true","measurements":[{"time_wall_clock":{"value":-1,"unit":"second"},"exit_code":0}]}]}`,
		`{"schema_version":2,"results":[{"command":"true","measurements":[{"time_wall_clock":{"value":"wrong","unit":"second"},"exit_code":0}]}]}`,
		`{"schema_version":2,"results":[{"command":"true","measurements":[{"time_wall_clock":{"value":0.001,"unit":"second"},"exit_code":1}]}]}`,
		`{"schema_version":2,"results":[{"command":"true","measurements":[]}]}`,
	} {
		t.Run(data, func(t *testing.T) {
			body := `case "$*" in --version) echo 'hyperfine 2.0.0';; *) while test "$1" != --export-json; do shift; done; shift; printf '%s\n' '` + data + `' > "$1";; esac`
			out, err := runQualityUtilitiesSmoke(t, "", map[string]string{"hyperfine": body})
			if err == nil || !strings.Contains(out, "Quality utilities smoke failed: hyperfine JSON assertion (exit ") {
				t.Fatalf("invalid benchmark accepted or lost diagnostic: %v\n%s", err, out)
			}
		})
	}
}

func TestQualityUtilitiesSmokeJSONParserFailure(t *testing.T) {
	out, err := runQualityUtilitiesSmoke(t, "", map[string]string{"jq": `exit 23`})
	if err == nil || !strings.Contains(out, "Quality utilities smoke failed: hyperfine JSON assertion (exit 23)") {
		t.Fatalf("JSON parser failure passed or lost status: %v\n%s", err, out)
	}
}
