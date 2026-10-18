package shelllayer

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
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
		"other option":   "gx\tgit log --grep='x y'\n",
		"quoted word":    "gx\tgit log '%h'\n",
		"dollar":         "gx\tgit log --pretty='%h $(id)'\n",
		"backtick":       "gx\tgit log --pretty='`id`'\n",
		"backslash":      "gx\tgit log --pretty='a\\b'\n",
		"double quote":   "gx\tgit log --pretty='a\"b'\n",
		"semicolon":      "gx\tgit log --pretty='%h;id'\n",
		"unterminated":   "gx\tgit log --pretty='%h\n",
		"trailing text":  "gx\tgit log --pretty='%h'x\n",
		"empty quotes":   "gx\tgit log --pretty=''\n",
		"quote in rg":    "gx\trg --pretty='%h'\n",
		"quote double":   "gx\tgit log --pretty='a' 'b'\n",
		"space in quote": "gx\tgit log --pretty='a'  --stat\n",
		"git config":     "gx\tgit config --format='x y'\n",
		"before log":     "gx\tgit --format='a' log\n",
		"after dashdash": "gx\tgit log -- --pretty='a b'\n",
		"two quoted":     "gx\tgit log --pretty='a' --format='b'\n",
		"git show":       "gx\tgit show --pretty='a b'\n",
	} {
		if _, err := Parse([]byte(row)); err == nil {
			t.Errorf("%s: expected rejection of %q", name, row)
		}
	}
}

// A git log --pretty/--format value may be single-quoted so a format string with
// spaces and placeholders stays one word. Nothing else may be quoted.
func TestParseAcceptsQuotedFormatValues(t *testing.T) {
	for row, want := range map[string][]string{
		"gx\tgit log --pretty='%h %s'\n":                                {"git", "log", "--pretty=%h %s"},
		"gx\tgit log --graph --pretty='%C(auto)%d %s <%an>'\n":          {"git", "log", "--graph", "--pretty=%C(auto)%d %s <%an>"},
		"gx\tgit log --pretty=format:'%C(yellow)%h %ad' --date=short\n": {"git", "log", "--pretty=format:%C(yellow)%h %ad", "--date=short"},
		"gx\tgit log --format=tformat:'%h'\n":                           {"git", "log", "--format=tformat:%h"},
	} {
		shortcuts, err := Parse([]byte(row))
		if err != nil {
			t.Errorf("%q: %v", row, err)
			continue
		}
		if got := shortcuts[0].Words(); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: words %q, want %q", row, got, want)
		}
	}
}

func TestTableCarriesTheGitLogFormatShortcuts(t *testing.T) {
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]string{}
	for _, s := range shortcuts {
		have[s.Name] = s.Command
	}
	for name, want := range map[string]string{
		"glol": "git log --graph --pretty='%Cred%h%Creset -%C(auto)%d%Creset %s %Cgreen(%ar) %C(bold blue)<%an>%Creset'",
		"gloo": "git log --pretty=format:'%C(yellow)%h %Cred%ad %Cblue%an%Cgreen%d %Creset%s' --date=short",
	} {
		if have[name] != want {
			t.Errorf("%s = %q, want %q", name, have[name], want)
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
	words, err := splitWords(r.Replace(s.Command))
	if err != nil {
		panic(err)
	}
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

// TestNushellProtectedListCoversInstalledNu requires every command name of the
// Nushell on this machine (built-in, keyword and the custom commands nu ships
// itself, such as pwd and banner) to be in the hand-written protected list, so
// a shortcut cannot take any of them. It skips where nu is not installed.
func TestNushellProtectedListCoversInstalledNu(t *testing.T) {
	nu, err := exec.LookPath("nu")
	if err != nil {
		t.Skip("nu not installed")
	}
	out, err := exec.Command(nu, "--no-config-file", "-c",
		`scope commands | where type in [built-in keyword custom] | get name | where {|n| $n !~ "[^a-z0-9]"} | str join "\n"`).Output()
	if err != nil {
		t.Fatal(err)
	}
	names := strings.Fields(string(out))
	if len(names) < 100 {
		t.Fatalf("unexpected nu command list (%d names)", len(names))
	}
	for _, n := range names {
		if !nushellBuiltins[n] {
			t.Errorf("nu command %q is not in nushellBuiltins", n)
		}
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
// because the base image provides them. Every entry says why the shortcut that
// runs it is safe; a new entry needs the same justification.
var baseCommands = map[string]string{
	"git":   "version control; the curated table omits irreversible clean and reset shortcuts",
	"df":    "df -h only reports disk usage and writes nothing",
	"du":    "du -h -d 1 only reports directory sizes and writes nothing",
	"free":  "free -h only reports memory usage and writes nothing",
	"mkdir": "mkdir -p only creates directories, never overwrites or removes anything",
	"rmdir": "rmdir removes only an empty directory and refuses a non-empty one",
}

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
	if !shipped["lsd"] || !shipped["jj"] || !shipped["rg"] || !shipped["fd"] {
		t.Fatalf("packages.txt not read correctly: %v", shipped)
	}
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shortcuts {
		if first := s.Words()[0]; baseCommands[first] == "" && !shipped[first] {
			t.Errorf("%s runs %q, which is neither a base command nor in packages.txt", s.Name, first)
		}
	}
}

// everydayShortcuts pins the curated everyday families: search, disk and
// memory, and directories. All are read-only or non-destructive and use only
// flags GNU coreutils and busybox both accept.
var everydayShortcuts = map[string]string{
	"rgi": "rg -i",
	"rgl": "rg -l",
	"rgf": "rg --files",
	"fdh": "fd --hidden",
	"dfh": "df -h",
	"duh": "du -h -d 1",
	"fr":  "free -h",
	"md":  "mkdir -p",
	"rd":  "rmdir",
}

// TestEverydayShortcutsBehaveTheSameInEveryShell requires the everyday
// families in the table and the exact same argv from each installed shell.
func TestEverydayShortcutsBehaveTheSameInEveryShell(t *testing.T) {
	all, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	var picked []Shortcut
	for name, command := range everydayShortcuts {
		found := false
		for _, s := range all {
			if s.Name == name {
				found = true
				if s.Command != command {
					t.Errorf("%s is %q, want %q", name, s.Command, command)
				}
				picked = append(picked, s)
			}
		}
		if !found {
			t.Errorf("shortcut %s is missing from the table", name)
		}
	}
	if len(picked) != len(everydayShortcuts) {
		t.FailNow()
	}
	dir := layerDir(t)
	bin := stubs(t, false)
	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.name); err != nil {
				t.Skipf("%s not installed", sh.name)
			}
			got := runShell(t, sh, dir, bin, picked)
			for _, s := range picked {
				w := strings.Fields(s.Command)
				want := "ARGV " + w[0] + "|" + strings.Join(append(w[1:], "EXTRA"), "|")
				if got[s.Name] != want {
					t.Errorf("%s: %s ran %q, want %q", sh.name, s.Name, got[s.Name], want)
				}
			}
		})
	}
}

// shadowExemptions are shortcut names that deliberately match an executable
// on some machines. Each entry needs a reason; keep it empty unless a name is
// a well-known Git or Jujutsu abbreviation.
var shadowExemptions = map[string]string{}

// TestShortcutNamesDoNotShadowExecutables fails when a shortcut name is also
// an executable on PATH during the test or a binary the toolbox catalog ships.
func TestShortcutNamesDoNotShadowExecutables(t *testing.T) {
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shortcuts {
		if _, ok := shadowExemptions[s.Name]; ok {
			continue
		}
		if p, err := exec.LookPath(s.Name); err == nil {
			t.Errorf("shortcut %s shadows the executable %s", s.Name, p)
		}
	}
	catalog, err := os.ReadFile(filepath.Join("..", "..", "bundles", "toolbox", "packages.catalog.json"))
	if err != nil {
		t.Skipf("toolbox catalog not readable: %v", err)
	}
	shipped := map[string]bool{}
	for _, m := range regexp.MustCompile(`"bin/([A-Za-z0-9._-]+)"`).FindAllStringSubmatch(string(catalog), -1) {
		shipped[m[1]] = true
	}
	if !shipped["rg"] || !shipped["fd"] {
		t.Fatalf("catalog bin list looks wrong: %v", shipped)
	}
	for _, s := range shortcuts {
		if _, ok := shadowExemptions[s.Name]; !ok && shipped[s.Name] {
			t.Errorf("shortcut %s shadows toolbox binary bin/%s", s.Name, s.Name)
		}
	}
}

// navCases are the navigation names with the directory each must reach from
// $T/a/b/c; "-" runs after cd $T/x, cd $T/a/b/c, so it returns to x (not the parent).
var navCases = []struct{ name, want string }{
	{"..", "a/b"},
	{"...", "a"},
	{"....", "."},
	{"-", "x"},
}

// navScript returns the command line that loads the layer, enters the temp
// tree as described above, runs name and prints "P=<pwd>".
func navScript(sh shell, layer, root, name string, layerSupported bool) []string {
	pre := []string{"cd " + root + "/x", "cd " + root + "/a/b/c"}
	switch sh.name {
	case "bash", "zsh":
		// Aliases expand at parse time, so the calls live in a second file.
		calls := layer + ".nav"
		body := strings.Join(append(pre, name, `printf 'P=%s\n' "$PWD"`), "\n") + "\n"
		if err := os.WriteFile(calls, []byte(body), 0o644); err != nil {
			panic(err)
		}
		if sh.name == "bash" {
			return []string{"bash", "--norc", "--noprofile", "-i", "-c", "source " + layer + "; source " + calls}
		}
		return []string{"zsh", "-f", "-i", "-c", "source " + layer + "; source " + calls}
	case "fish":
		lines := []string{"source " + layer}
		lines = append(lines, pre...)
		if layerSupported {
			// Abbreviations expand at the prompt: run the expansion text.
			lines = append(lines,
				"set -l exp (abbr --show | string match -r -- '^abbr .*-- "+regexp.QuoteMeta(name)+" (.*)$')[2]",
				"eval set -l exp $exp", `eval "$exp"`)
		} else {
			lines = append(lines, name)
		}
		lines = append(lines, "echo P=$PWD")
		return []string{"fish", "--no-config", "--interactive", "-c", strings.Join(lines, "\n")}
	}
	panic("nu navigation runs in a pseudo-terminal: see nuREPL")
}

// nuREPL types lines into an interactive Nushell on a pseudo-terminal
// provided by script(1), from the directory dir, and returns what it printed.
// Nushell's implicit cd (typing `..` alone) exists only in the interactive
// REPL, never under `nu -c`, so measuring it needs a terminal. Each line is
// sent after the previous command finished, which the terminal integration
// marks with an OSC 133;D sequence. The REPL asks the terminal for the cursor
// position and discards typed input until it hears back, so the helper answers
// like a terminal does.
func nuREPL(t *testing.T, dir string, lines []string) string {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command("script", "-qec", "stty cols 500 rows 50; exec nu --no-config-file --no-history", "/dev/null")
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_CONFIG_HOME=" + home,
		"XDG_DATA_HOME=" + home, "TERM=xterm", "BDOCK_STARSHIP=0",
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var out bytes.Buffer
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pipe.Read(buf)
			if bytes.Contains(buf[:n], []byte("\x1b[6n")) {
				io.WriteString(in, "\x1b[1;1R")
			}
			mu.Lock()
			out.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	count := func(marker string) int {
		mu.Lock()
		defer mu.Unlock()
		return strings.Count(out.String(), marker)
	}
	waitFor := func(marker string, n int) {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) && count(marker) < n {
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitFor("\x1b]133;B", 1) // first prompt is up
	done := 0
	for _, l := range lines {
		io.WriteString(in, l+"\r")
		done++
		waitFor("\x1b]133;D", done)
	}
	io.WriteString(in, "exit\r")
	in.Close()
	finished := make(chan struct{})
	go func() { cmd.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		<-finished
	}
	mu.Lock()
	defer mu.Unlock()
	return regexp.MustCompile("\\x1b\\[[0-9;?]*[a-zA-Z]|\\x1b\\][^\\x1b\\x07]*(\\x07|\\x1b\\\\)|\\x1b[=>78]").ReplaceAllString(strings.ReplaceAll(out.String(), "\r", "\n"), "")
}

// navTree makes the temporary a/b/c tree and returns its real path.
func navTree(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a/b/c", "x"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// navPWD returns the directory a P=<pwd> line reports for the given tag.
func navPWD(out, tag string) string {
	got := ""
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, tag+"=") {
			got = strings.TrimPrefix(line, tag+"=")
		}
	}
	return got
}

// TestNavigationShortcutsChangeDirectoryInEveryShell runs the real cd through
// every navigation name in each installed shell, in a temporary a/b/c tree,
// and asserts the resulting pwd. Per-shell expectations come from
// NavigationSupport, which is the table in docs/toolbox.md: native and
// layer-defined names must reach the directory; unsupported names must leave
// it unchanged (the layer does not fake them). A shell that is not installed
// is skipped with a message, which is not a pass.
func TestNavigationShortcutsChangeDirectoryInEveryShell(t *testing.T) {
	dir := layerDir(t)
	bin := t.TempDir()
	for _, sh := range shells {
		if _, err := exec.LookPath(sh.name); err != nil {
			t.Run(sh.name, func(t *testing.T) { t.Skipf("%s not installed: navigation NOT verified", sh.name) })
			continue
		}
		var nuOut string
		var nuRoot string
		if sh.name == "nu" {
			if _, err := exec.LookPath("script"); err != nil {
				t.Run(sh.name, func(t *testing.T) { t.Skip("script(1) not installed: nu navigation NOT verified") })
				continue
			}
			nuRoot = navTree(t)
			lines := []string{"source " + filepath.Join("share", "nushell", "vendor", "autoload", "bdock-shell.nu")}
			for _, c := range navCases {
				lines = append(lines, "cd "+nuRoot+"/x", "cd "+nuRoot+"/a/b/c", c.name, `print $"P`+c.name+`=($env.PWD)"`)
			}
			nuOut = nuREPL(t, dir, lines)
		}
		for _, c := range navCases {
			t.Run(sh.name+"/"+c.name, func(t *testing.T) {
				state := NavigationSupport(sh.name, c.name)
				root, got := nuRoot, ""
				if sh.name == "nu" {
					got = navPWD(nuOut, "P"+c.name)
				} else {
					root = navTree(t)
					got = navPWD(runLenient(t, navScript(sh, filepath.Join(dir, sh.file), root, c.name, state == "layer"), bin), "P")
				}
				want := filepath.Join(root, c.want)
				switch state {
				case "native", "layer":
				case "unsupported":
					want = filepath.Join(root, "a", "b", "c")
				default:
					t.Fatalf("no support state for %s %s: %q", sh.name, c.name, state)
				}
				if got != want {
					t.Errorf("%s %s (%s): pwd %q, want %q", sh.name, c.name, state, got, want)
				}
			})
		}
	}
}

// runLenient is run without failing on a non-zero exit: an unsupported name
// is a command-not-found error in some shells.
func runLenient(t *testing.T, argv []string, bin string) string {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home,
		"TERM=dumb", "BDOCK_STARSHIP=0",
	}
	cmd.Stdin = strings.NewReader("")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// TestNavigationTableIsPinned records the measured capability table.
func TestNavigationTableIsPinned(t *testing.T) {
	want := map[string]map[string]NavState{
		"bash": {"..": "layer", "...": "layer", "....": "layer", "-": "layer"},
		"zsh":  {"..": "layer", "...": "layer", "....": "layer", "-": "unsupported"},
		"fish": {"..": "native", "...": "layer", "....": "layer", "-": "layer"},
		"nu":   {"..": "native", "...": "native", "....": "native", "-": "unsupported"},
	}
	for sh, names := range want {
		for name, state := range names {
			if got := NavigationSupport(sh, name); got != state {
				t.Errorf("%s %s: %q, want %q", sh, name, got, state)
			}
		}
	}
	nav := Navigation()
	if len(nav) != 4 {
		t.Fatalf("want 4 navigation shortcuts, got %v", nav)
	}
	for i, c := range navCases {
		if nav[i].Name != c.name {
			t.Errorf("navigation[%d] is %q, want %q", i, nav[i].Name, c.name)
		}
	}
}

// TestNavigationLeavesOrdinaryValidationStrict proves that adding navigation
// names did not loosen what an ordinary shortcut row may contain.
func TestNavigationLeavesOrdinaryValidationStrict(t *testing.T) {
	for name, row := range map[string]string{
		"dotdot name":      "..\tgit status\n",
		"dots name":        "...\tgit status\n",
		"dash name":        "-\tgit status\n",
		"cd command":       "up\tcd ..\n",
		"parent word":      "gx\tgit -C ..\n",
		"parent path word": "gx\tgit show ../../x\n",
		"bare dash word":   "gx\tgit checkout -\n",
		"dot word":         "gx\tgit add .\n",
	} {
		if _, err := Parse([]byte(row)); err == nil {
			t.Errorf("%s: expected rejection of %q", name, row)
		}
	}
	for _, s := range Navigation() {
		if validName.MatchString(s.Name) {
			t.Errorf("navigation name %q must not be an ordinary valid name", s.Name)
		}
	}
}

func TestNavigationDefinitionsMatchTheSupportTable(t *testing.T) {
	gen := Generate(nil)
	for _, c := range []struct{ file, shell string }{{BashPath, "bash"}, {ZshPath, "zsh"}, {FishPath, "fish"}, {NuPath, "nu"}} {
		text := string(gen[c.file])
		for _, c2 := range navCases {
			state := NavigationSupport(c.shell, c2.name)
			var line string
			switch c.shell {
			case "bash", "zsh":
				line = "alias -- " + c2.name + "='cd "
			case "fish":
				line = "abbr --add --global -- " + c2.name + " 'cd "
			case "nu":
				line = "alias " + c2.name + " ="
			}
			if has := strings.Contains(text, line); has != (state == "layer") {
				t.Errorf("%s %s: state %s but definition present=%v", c.shell, c2.name, state, has)
			}
		}
	}
}

// TestNavigationCapabilityTableInDocsMatchesGenerator parses the capability
// table in docs/toolbox.md and requires every cell to agree with what the
// generator emits: "defined by the layer" exactly where the generated file
// defines the name, and "native" or "unsupported" exactly where it does not
// (NavigationSupport records which of the two, and the real-shell test above
// proves it).
func TestNavigationCapabilityTableInDocsMatchesGenerator(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "toolbox.md"))
	if err != nil {
		t.Fatal(err)
	}
	cellState := map[string]NavState{
		"native": NavNative, "defined by the layer": NavLayer, "unsupported": NavUnsupported,
	}
	header := "| Shell (version) | `..` | `...` | `....` | `-` |"
	lines := strings.Split(string(doc), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == header {
			start = i
		}
	}
	if start < 0 {
		t.Fatalf("docs/toolbox.md has no capability table with header %q", header)
	}
	gen := Generate(nil)
	files := map[string]string{"bash": BashPath, "zsh": ZshPath, "fish": FishPath, "nu": NuPath}
	seen := map[string]bool{}
	for _, l := range lines[start+2:] {
		if !strings.HasPrefix(l, "|") {
			break
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(l), "|"), "|")
		if len(cells) != 5 {
			t.Fatalf("capability row %q: want 5 cells", l)
		}
		shellName := strings.ToLower(strings.Fields(cells[0])[0])
		if shellName == "nushell" {
			shellName = "nu"
		}
		file, ok := files[shellName]
		if !ok {
			t.Fatalf("capability row for unknown shell %q", shellName)
		}
		seen[shellName] = true
		text := string(gen[file])
		for i, s := range Navigation() {
			state, ok := cellState[strings.TrimSpace(cells[i+1])]
			if !ok {
				t.Fatalf("%s %s: unknown state %q", shellName, s.Name, cells[i+1])
			}
			var def string
			switch shellName {
			case "bash", "zsh":
				def = "alias -- " + s.Name + "='" + s.Command + "'\n"
			case "fish":
				def = "abbr --add --global -- " + s.Name + " '" + s.Command + "'\n"
			case "nu":
				def = "alias " + s.Name + " ="
			}
			if has := strings.Contains(text, def); has != (state == NavLayer) {
				t.Errorf("%s %s: docs say %q but generated definition present=%v", shellName, s.Name, cells[i+1], has)
			}
			if got := NavigationSupport(shellName, s.Name); got != state {
				t.Errorf("%s %s: docs say %q, NavigationSupport says %q", shellName, s.Name, state, got)
			}
		}
	}
	for name := range files {
		if !seen[name] {
			t.Errorf("capability table has no row for %s", name)
		}
	}
}
