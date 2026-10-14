package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelixSmokeChecksParserAndQueries(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/helix-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"", "styled", "parser", "queries", "styled-parser", "styled-queries", "command"} {
		t.Run(missing, func(t *testing.T) {
			home := t.TempDir()
			fake := filepath.Join(home, "hx")
			data := "#!/bin/sh\ncase \"$*\" in\n--version) echo 'helix 25.07.1' ;;\n--health) echo \"Config file: $HOME/.config/helix/config.toml\" ;;\n'--health go')\n"
			if !strings.HasSuffix(missing, "parser") {
				data += "printf 'Tree-sitter parser: "
				if strings.HasPrefix(missing, "styled") {
					data += "\\033[38;5;10m✓\\033[39m"
				} else {
					data += "✓"
				}
				data += "\\n'\n"
			} else {
				data += "echo 'Tree-sitter parser: None'\n"
			}
			if !strings.HasSuffix(missing, "queries") {
				data += "printf 'Highlight queries: "
				if strings.HasPrefix(missing, "styled") {
					data += "\\033[38;5;10m✓\\033[39m"
				} else {
					data += "✓"
				}
				data += "\\n'\n"
			} else {
				data += "echo 'Highlight queries: ✘'\n"
			}
			if missing == "command" {
				data += "echo 'synthetic health command failed'; exit 7\n"
			}
			data += ";;\n*) exit 1 ;;\nesac\n"
			if err := os.WriteFile(fake, []byte(data), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script, fake)
			cmd.Env = append(os.Environ(), "HOME="+home)
			out, err := cmd.CombinedOutput()
			valid := missing == "" || missing == "styled"
			if valid && err != nil || !valid && err == nil {
				t.Fatalf("missing=%s error=%v output=%s", missing, err, out)
			}
			if !valid && (!strings.Contains(string(out), "Helix smoke failed at") || !strings.Contains(string(out), "Helix report:")) {
				t.Fatalf("failure report was hidden: %s", out)
			}
			if missing == "command" && (!strings.Contains(string(out), "exit 7") || !strings.Contains(string(out), "synthetic health command failed")) {
				t.Fatalf("command failure status/output was lost: %s", out)
			}
		})
	}
}
