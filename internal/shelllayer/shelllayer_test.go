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
		"stock ls":       "gx\tls -l\n",
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

// stubs puts a fake executable for the first word of every table command
// (git, jj, lsd, ...) first on PATH. Each prints the argv it received, and the
// git stub also answers the two branch lookups the layer performs.
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
	bodies := map[string]string{"git": git}
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shortcuts {
		name := s.Words()[0]
		if _, ok := bodies[name]; !ok {
			bodies[name] = "#!/bin/sh\nprintf 'ARGV " + name + "'; printf '|%s' \"$@\"; printf '\\n'\n"
		}
	}
	for name, body := range bodies {
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

// runShell runs every shortcut once with the argument EXTRA and returns the
// ARGV line each one printed, keyed by shortcut name.
func runShell(t *testing.T, sh shell, dir, bin string, shortcuts []Shortcut) map[string]string {
	t.Helper()
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
	return got
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
			got := runShell(t, sh, dir, bin, shortcuts)
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

// lsdShortcuts pins the curated listing family. Every command runs lsd, which
// the toolbox ships; stock ls is never aliased. Flags come from the lsd 1.2.0
// documentation (doc/lsd.md), not from a measured run.
var lsdShortcuts = map[string]string{
	"ll":    "lsd -l --color auto",
	"la":    "lsd -A --color auto",
	"lla":   "lsd -lA --color auto",
	"lt":    "lsd -lt --color auto",
	"ltr":   "lsd -ltr --color auto",
	"l1":    "lsd -1 --color auto",
	"ltree": "lsd --tree --color auto",
	"llg":   "lsd -l --group-dirs first --color auto",
}

// TestLsdShortcutsBehaveTheSameInEveryShell requires the lsd family in the table
// and the exact same argv, extra arguments appended, from each installed shell.
func TestLsdShortcutsBehaveTheSameInEveryShell(t *testing.T) {
	all, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	var ls []Shortcut
	for name, command := range lsdShortcuts {
		found := false
		for _, s := range all {
			if s.Name == name {
				found = true
				if s.Command != command {
					t.Errorf("%s is %q, want %q", name, s.Command, command)
				}
				ls = append(ls, s)
			}
		}
		if !found {
			t.Errorf("shortcut %s is missing from the table", name)
		}
	}
	if len(ls) != len(lsdShortcuts) {
		t.FailNow()
	}
	dir := layerDir(t)
	bin := stubs(t, false)
	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.name); err != nil {
				t.Skipf("%s not installed", sh.name)
			}
			got := runShell(t, sh, dir, bin, ls)
			for _, s := range ls {
				want := "ARGV lsd|" + strings.Join(append(strings.Fields(s.Command)[1:], "EXTRA"), "|")
				if got[s.Name] != want {
					t.Errorf("%s: %s ran %q, want %q", sh.name, s.Name, got[s.Name], want)
				}
			}
		})
	}
}

// Names that are Nushell built-in or keyword commands. An alias with one of
// these names would replace the built-in for the whole session; Nushell's ls,
// for instance, returns a table that pipelines depend on. cat is not a Nushell
// built-in and stays available as a name.
var nushellProtected = []string{
	"ls", "cd", "cp", "mv", "rm", "mkdir", "open", "save", "echo", "touch",
	"du", "ps", "kill", "which", "start", "sleep", "glob", "find", "sort",
	"uniq", "first", "last", "get", "select", "date", "help", "exit",
	"alias", "def", "use", "source", "if", "for", "let", "mut", "try",
}

func TestParseRejectsShadowingNushellBuiltins(t *testing.T) {
	for _, name := range nushellProtected {
		if _, err := Parse([]byte(name + "\tgit status\n")); err == nil {
			t.Errorf("expected rejection of shortcut named %q", name)
		}
	}
	if _, err := Parse([]byte("cat\tgit status\n")); err != nil {
		t.Errorf("cat is not a Nushell built-in, got %v", err)
	}
}

// TestTableNamesAvoidInstalledNushellBuiltins checks the table against the
// built-ins of the Nushell on this machine, which the catalog pins.
func TestTableNamesAvoidInstalledNushellBuiltins(t *testing.T) {
	nu, err := exec.LookPath("nu")
	if err != nil {
		t.Skip("nu not installed")
	}
	out, err := exec.Command(nu, "--no-config-file", "-c",
		`scope commands | where type in [built-in keyword] | get name | where {|n| $n !~ " "} | str join "\n"`).Output()
	if err != nil {
		t.Fatal(err)
	}
	builtin := map[string]bool{}
	for _, n := range strings.Fields(string(out)) {
		builtin[n] = true
	}
	if !builtin["ls"] || !builtin["cd"] {
		t.Fatalf("unexpected nu command list: %q", out)
	}
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shortcuts {
		if builtin[s.Name] {
			t.Errorf("shortcut %s shadows a Nushell built-in", s.Name)
		}
	}
	for _, name := range nushellProtected {
		if !builtin[name] {
			t.Errorf("%s is listed as protected but is not a Nushell built-in", name)
		}
	}
	// The pinned Nushell must not have grown built-ins the parser does not
	// know; update nushellBuiltins when the catalog moves to a new release.
	for name := range builtin {
		if _, err := Parse([]byte(name + "\tgit status\n")); err == nil {
			t.Errorf("Parse accepts %q, a built-in of the installed Nushell; add it to nushellBuiltins", name)
		}
	}
}

// baseCommands are first words the toolbox does not list in packages.txt
// because the base image provides them.
var baseCommands = map[string]bool{"git": true}

// TestShortcutCommandsAreShippedTools requires every first word of every table
// command to be a base command or a tool named in the toolbox package list, so
// a shortcut never points at a tool the toolbox does not ship.
func TestShortcutCommandsAreShippedTools(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "bundles", "toolbox", "packages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	shipped := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.IndexAny(line, "<>=!,~ "); i >= 0 {
			line = line[:i]
		}
		shipped[line] = true
	}
	if !shipped["lsd"] || !shipped["jj"] {
		t.Fatalf("packages.txt not read correctly: %v", shipped)
	}
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shortcuts {
		if first := s.Words()[0]; !baseCommands[first] && !shipped[first] {
			t.Errorf("%s runs %q, which is neither a base command nor in packages.txt", s.Name, first)
		}
	}
}
