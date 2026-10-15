package shelllayer

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite generated/ from shortcuts.tsv")

func TestTableIsValid(t *testing.T) {
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	if len(shortcuts) < 50 {
		t.Fatalf("expected the curated table, got %d shortcuts", len(shortcuts))
	}
}

func TestParseRejectsUnsafeRows(t *testing.T) {
	for name, row := range map[string]string{
		"bang name":      "gc!\tgit commit\n",
		"uppercase name": "gC\tgit commit\n",
		"duplicate":      "gst\tgit status\ngst\tgit status -s\n",
		"other command":  "x\trm -rf /\n",
		"quote":          "gcm\tgit commit -m 'x'\n",
		"substitution":   "gx\tgit $(id)\n",
		"chain":          "gx\tgit fetch ; git merge\n",
		"double space":   "gx\tgit  fetch\n",
		"no command":     "gx\n",
	} {
		if _, err := Parse([]byte(row)); err == nil {
			t.Errorf("%s: expected rejection of %q", name, row)
		}
	}
}

func TestGeneratedFilesAreCurrent(t *testing.T) {
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	want := Generate(shortcuts)
	for _, p := range []string{BashPath, ZshPath, FishPath, NuPath} {
		file := filepath.Join("generated", generatedName(p))
		if *update {
			if err := os.WriteFile(file, want[p], 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want[p]) {
			t.Errorf("%s is stale; run: go test ./internal/shelllayer -run TestGeneratedFilesAreCurrent -update", file)
		}
	}
}

func TestFilesCoverLayerAndNotices(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{BashPath, ZshPath, FishPath, NuPath, TablePath, "licenses/shell-layer/plugin-git-LICENSE", "licenses/shell-layer/SOURCE"} {
		if len(files[p]) == 0 {
			t.Errorf("missing %s", p)
		}
	}
	if !bytes.Contains(files["licenses/shell-layer/plugin-git-LICENSE"], []byte("The MIT License")) {
		t.Error("plugin-git MIT notice not retained")
	}
}

// stubs puts fake git and jj executables first on PATH. They print the argv
// they received, and answer the two branch lookups the layer performs.
func stubs(t *testing.T, detached bool) string {
	t.Helper()
	dir := t.TempDir()
	head := `printf '%s\n' feat`
	if detached {
		head = "exit 1"
	}
	git := `#!/bin/sh
case "$*" in
"symbolic-ref --quiet --short HEAD") ` + head + ` ;;
"symbolic-ref --quiet --short refs/remotes/origin/HEAD") printf '%s\n' origin/trunk ;;
*) printf 'ARGV git'; printf '|%s' "$@"; printf '\n' ;;
esac
`
	jj := "#!/bin/sh\nprintf 'ARGV jj'; printf '|%s' \"$@\"; printf '\\n'\n"
	for name, body := range map[string]string{"git": git, "jj": jj} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// expected is the argv each shortcut must produce when invoked with "EXTRA".
func expected(s Shortcut) string {
	r := strings.NewReplacer(branchToken, "feat", defaultToken, "trunk")
	words := strings.Fields(r.Replace(s.Command))
	return "ARGV " + words[0] + "|" + strings.Join(append(words[1:], "EXTRA"), "|")
}

type shell struct {
	name   string
	file   string
	script func(file string, shortcuts []Shortcut) []string
}

var shells = []shell{
	// bash and zsh expand aliases while parsing, so the calls live in a second
	// file sourced after the layer, as an rc file and the prompt would be.
	{"bash", BashPath, func(file string, s []Shortcut) []string {
		return []string{"bash", "--norc", "--noprofile", "-i", "-c", "source " + file + "; source " + posixCalls(file, s)}
	}},
	{"zsh", ZshPath, func(file string, s []Shortcut) []string {
		return []string{"zsh", "-f", "-i", "-c", "source " + file + "; source " + posixCalls(file, s)}
	}},
	{"fish", FishPath, func(file string, s []Shortcut) []string {
		// Abbreviations only expand at the prompt, so expand them the way the
		// prompt would: take each one's expansion text and run it.
		lines := []string{"source " + file}
		for _, x := range s {
			if x.Dynamic() {
				lines = append(lines, "echo @@"+x.Name, x.Name+" EXTRA")
				continue
			}
			lines = append(lines,
				"echo @@"+x.Name,
				"set -l exp (abbr --show | string match -r -- '^abbr .*-- "+x.Name+" (.*)$')[2]",
				"eval set -l exp $exp",
				`eval "$exp EXTRA"`)
		}
		return []string{"fish", "--no-config", "--interactive", "-c", strings.Join(lines, "\n")}
	}},
	{"nu", NuPath, func(file string, s []Shortcut) []string {
		lines := []string{"source " + file}
		for _, x := range s {
			lines = append(lines, "print @@"+x.Name, x.Name+" EXTRA")
		}
		return []string{"nu", "--no-config-file", "--no-history", "-c", strings.Join(lines, "\n")}
	}},
}

// posixCalls writes "echo @@name; name EXTRA" lines next to the layer file.
func posixCalls(file string, s []Shortcut) string {
	var b strings.Builder
	for _, x := range s {
		fmt.Fprintf(&b, "echo @@%s\n%s EXTRA\n", x.Name, x.Name)
	}
	path := file + "." + fmt.Sprint(len(s)) + ".calls"
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
	return path
}

func layerDir(t *testing.T) string {
	t.Helper()
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for p, data := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func run(t *testing.T, argv []string, bin string) string {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home,
		"TERM=dumb", "BDOCK_STARSHIP=0",
	}
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("%s failed: %v\n%s\n%s", argv[0], err, out, stderr)
	}
	return string(out)
}

// TestShortcutsBehaveTheSameInEveryShell runs every shortcut in every shell
// present on this machine and requires the exact same argv from each. Shells
// that are not installed are skipped; the toolbox smoke test runs fish and nu.
func TestShortcutsBehaveTheSameInEveryShell(t *testing.T) {
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	dir := layerDir(t)
	bin := stubs(t, false)
	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.name); err != nil {
				t.Skipf("%s not installed", sh.name)
			}
			out := run(t, sh.script(filepath.Join(dir, sh.file), shortcuts), bin)
			got := map[string]string{}
			current := ""
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(line, "@@"):
					current = strings.TrimPrefix(line, "@@")
				case strings.HasPrefix(line, "ARGV ") && current != "":
					got[current] = line
					current = ""
				}
			}
			for _, s := range shortcuts {
				if got[s.Name] != expected(s) {
					t.Errorf("%s: %s ran %q, want %q", sh.name, s.Name, got[s.Name], expected(s))
				}
			}
		})
	}
}

// TestPosixLayerLoadsOverExistingAliases covers sourcing the layer after a
// framework such as Oh My Zsh defined the same names as aliases.
func TestPosixLayerLoadsOverExistingAliases(t *testing.T) {
	dir := layerDir(t)
	bin := stubs(t, false)
	for _, sh := range []struct {
		name, file string
		argv       []string
	}{
		{"bash", BashPath, []string{"bash", "--norc", "--noprofile", "-i", "-c"}},
		{"zsh", ZshPath, []string{"zsh", "-f", "-i", "-c"}},
	} {
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.name); err != nil {
				t.Skipf("%s not installed", sh.name)
			}
			calls := filepath.Join(t.TempDir(), "calls")
			if err := os.WriteFile(calls, []byte("echo @@gpsup\ngpsup EXTRA\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			script := "alias gpsup='echo framework'; alias bdock-git-branch='echo framework'; source " + filepath.Join(dir, sh.file) + "; unalias gpsup bdock-git-branch; source " + calls
			out := run(t, append(sh.argv, script), bin)
			if !strings.Contains(out, "ARGV git|push|--set-upstream|origin|feat|EXTRA") {
				t.Errorf("%s: layer did not load over existing aliases:\n%s", sh.name, out)
			}
		})
	}
}

// TestBranchShortcutsRefuseDetachedHead keeps gpsup from pushing a commit ID
// when HEAD is detached, as it is in most jj-colocated repositories.
func TestBranchShortcutsRefuseDetachedHead(t *testing.T) {
	dir := layerDir(t)
	bin := stubs(t, true)
	gpsup := []Shortcut{{"gpsup", "git push --set-upstream origin {branch}"}}
	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.name); err != nil {
				t.Skipf("%s not installed", sh.name)
			}
			argv := sh.script(filepath.Join(dir, sh.file), gpsup)
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Env = []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TERM=dumb", "BDOCK_STARSHIP=0"}
			out, _ := cmd.CombinedOutput()
			if !strings.Contains(string(out), "not on a branch") {
				t.Errorf("%s: no detached-HEAD error:\n%s", sh.name, out)
			}
			if strings.Contains(string(out), "ARGV git|push") {
				t.Errorf("%s: ran git push without a branch:\n%s", sh.name, out)
			}
		})
	}
}
