package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHelixSmokeChecksParserAndQueries(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/helix-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"", "parser", "queries"} {
		t.Run(missing, func(t *testing.T) {
			home := t.TempDir()
			fake := filepath.Join(home, "hx")
			data := "#!/bin/sh\ncase \"$*\" in\n--version) echo 'helix 25.07.1' ;;\n--health) echo \"Config file: $HOME/.config/helix/config.toml\" ;;\n'--health go')\n"
			if missing != "parser" {
				data += "echo 'Tree-sitter parser: ✓'\n"
			}
			if missing != "queries" {
				data += "echo 'Highlight queries: ✓'\n"
			}
			data += ";;\n*) exit 1 ;;\nesac\n"
			if err := os.WriteFile(fake, []byte(data), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script, fake)
			cmd.Env = append(os.Environ(), "HOME="+home)
			out, err := cmd.CombinedOutput()
			if missing == "" && err != nil || missing != "" && err == nil {
				t.Fatalf("missing=%s error=%v output=%s", missing, err, out)
			}
		})
	}
}
