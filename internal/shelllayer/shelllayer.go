// Package shelllayer generates BundleDock's shared shell layer: one shortcut
// table rendered as init files for bash, zsh, fish and Nushell, plus a
// guarded starship prompt. Every shell gets the same names, and each name runs
// the same command with the same arguments.
//
// The generated files are committed under generated/ and embedded; the toolbox
// build copies them verbatim. Regenerate with:
//
//	go test ./internal/shelllayer -run TestGeneratedFilesAreCurrent -update
package shelllayer

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed shortcuts.tsv
var table []byte

//go:embed generated
var generated embed.FS

//go:embed notices
var notices embed.FS

// Token values substituted into commands.
const (
	branchToken  = "{branch}"
	defaultToken = "{default}"
)

// Shortcut is one row of the table.
type Shortcut struct {
	Name    string
	Command string
}

// Dynamic reports whether the command needs a branch lookup at run time.
func (s Shortcut) Dynamic() bool {
	return strings.Contains(s.Command, branchToken) || strings.Contains(s.Command, defaultToken)
}

// Words splits the command; tokens stay as single words.
func (s Shortcut) Words() []string { return strings.Fields(s.Command) }

var (
	validName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	validWord = regexp.MustCompile(`^(\{branch\}|\{default\}|[A-Za-z0-9][A-Za-z0-9._/:=-]*|-[A-Za-z0-9-]+(=[A-Za-z0-9._/:-]+)?)$`)
)

// Shortcuts parses and validates the embedded table.
func Shortcuts() ([]Shortcut, error) { return Parse(table) }

// nushellBuiltins are the built-in, keyword and shipped custom commands (pwd,
// banner) of the Nushell release pinned in the catalog (0.116.1) whose names a
// shortcut could take. An alias
// with one of these names would replace the built-in for the whole Nushell
// session; `ls` in particular returns a table that pipelines rely on. Names
// with a dash cannot be shortcut names and are left out. A test compares the
// list with the installed nu, so a Nushell bump fails until it is refreshed;
// that test skips on machines without nu, so it protects only where nu exists.
var nushellBuiltins = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{
		"abbr", "alias", "all", "ansi", "any", "append", "ast", "attr",
		"banner", "bits", "break", "bytes", "cal", "cd", "char", "chunks", "clear",
		"collect", "columns", "combinations", "commandline", "compact",
		"complete", "config", "const", "continue", "cp", "date", "debug",
		"decode", "def", "default", "describe", "detect", "difference", "do",
		"drop", "du", "each", "echo", "encode", "enumerate", "error", "every",
		"exec", "exit", "explain", "explore", "export", "extern", "fill",
		"filter", "find", "first", "flatten", "for", "format", "from",
		"generate", "get", "glob", "grid", "hash", "headers", "help", "hide",
		"histogram", "history", "http", "idx", "if", "ignore", "input",
		"insert", "inspect", "interleave", "intersect", "into", "items",
		"job", "join", "keybindings", "kill", "last", "length", "let",
		"lines", "loop", "ls", "match", "math", "matrix", "merge", "metadata",
		"mkdir", "mktemp", "module", "move", "mut", "mv", "open", "overlay",
		"panic", "parse", "path", "peek", "permutations", "plugin", "port",
		"prepend", "print", "ps", "pwd", "query", "random", "reduce", "reject",
		"rename", "return", "reverse", "rm", "roll", "rotate", "run", "save",
		"schema", "scope", "select", "semver", "seq", "shuffle", "skip",
		"sleep", "slice", "sort", "source", "split", "start", "stor", "str",
		"sys", "table", "take", "tee", "term", "timeit", "to", "touch",
		"transpose", "try", "tui", "tutor", "ulimit", "umask", "uname",
		"union", "uniq", "unlet", "update", "upsert", "url", "use", "values",
		"version", "view", "watch", "where", "which", "while", "whoami",
		"window", "wrap", "zip",
	} {
		m[n] = true
	}
	return m
}()

// allowedCommands are the first words a shortcut may run: Git, Jujutsu, the
// lsd listing family, rg and fd searches, read-only df, du and free reports,
// and the non-destructive mkdir -p and rmdir (empty directories only). Stock
// ls is never a shortcut command.
var allowedCommands = map[string]bool{
	"git": true, "jj": true, "lsd": true, "rg": true, "fd": true,
	"df": true, "du": true, "free": true, "mkdir": true, "rmdir": true,
}

// Parse reads name<TAB>command rows, skipping blank lines and # comments.
// Names must be valid in every shell, unique and lowercase; command words are
// restricted to characters that need no quoting in any of the four shells.
func Parse(data []byte) ([]Shortcut, error) {
	var out []Shortcut
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.TrimSpace(text) == "" || strings.HasPrefix(text, "#") {
			continue
		}
		name, command, ok := strings.Cut(text, "\t")
		if !ok || strings.Contains(command, "\t") {
			return nil, fmt.Errorf("line %d: want name<TAB>command", line)
		}
		if !validName.MatchString(name) {
			return nil, fmt.Errorf("line %d: invalid shortcut name %q", line, name)
		}
		if nushellBuiltins[name] {
			return nil, fmt.Errorf("line %d: %q is a Nushell built-in; a shortcut must not shadow it", line, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("line %d: duplicate shortcut %q", line, name)
		}
		seen[name] = true
		words := strings.Fields(command)
		if len(words) == 0 || strings.Join(words, " ") != command {
			return nil, fmt.Errorf("line %d: %s: command must be single-spaced words", line, name)
		}
		if !allowedCommands[words[0]] {
			return nil, fmt.Errorf("line %d: %s: command must start with an allowed command", line, name)
		}
		for _, w := range words {
			if !validWord.MatchString(w) {
				return nil, fmt.Errorf("line %d: %s: unsupported word %q", line, name, w)
			}
		}
		out = append(out, Shortcut{name, command})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no shortcuts")
	}
	return out, nil
}

// Bundle paths of the generated files, relative to the toolbox root. fish and
// Nushell load theirs automatically when <toolbox>/share is in XDG_DATA_DIRS;
// bash and zsh source theirs from the system rc file.
const (
	BashPath  = "share/shell/init.bash"
	ZshPath   = "share/shell/init.zsh"
	FishPath  = "share/fish/vendor_conf.d/bdock-shell.fish"
	NuPath    = "share/nushell/vendor/autoload/bdock-shell.nu"
	TablePath = "share/shell/shortcuts.tsv"
)

// Generate renders every file of the layer from shortcuts.
func Generate(shortcuts []Shortcut) map[string][]byte {
	return map[string][]byte{
		BashPath:  []byte(renderBash(shortcuts)),
		ZshPath:   []byte(renderZsh(shortcuts)),
		FishPath:  []byte(renderFish(shortcuts)),
		NuPath:    []byte(renderNu(shortcuts)),
		TablePath: table,
	}
}

// Files returns the committed generated files keyed by bundle path, plus the
// retained plugin-git notice and its source record under licenses/.
func Files() (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, p := range []string{BashPath, ZshPath, FishPath, NuPath} {
		data, err := fs.ReadFile(generated, "generated/"+generatedName(p))
		if err != nil {
			return nil, err
		}
		out[p] = data
	}
	out[TablePath] = table
	for _, name := range []string{"plugin-git-LICENSE", "SOURCE"} {
		data, err := fs.ReadFile(notices, "notices/"+name)
		if err != nil {
			return nil, err
		}
		out["licenses/shell-layer/"+name] = data
	}
	return out, nil
}

// generatedName maps a bundle path to its flat file name under generated/.
func generatedName(bundlePath string) string {
	switch bundlePath {
	case BashPath:
		return "init.bash"
	case ZshPath:
		return "init.zsh"
	case FishPath:
		return "bdock-shell.fish"
	case NuPath:
		return "bdock-shell.nu"
	}
	panic("unknown generated path " + bundlePath)
}

const header = "BundleDock shared shell layer. Generated from internal/shelllayer/shortcuts.tsv; do not edit."

// Static shortcuts become aliases (bash, zsh, Nushell) or abbreviations
// (fish). Shortcuts naming {branch} or {default} become functions that resolve
// the branch first and stop before running anything when that fails, so a
// detached HEAD never turns into a push of an empty or commit-ID refspec.

// posixFunction renders a dynamic shortcut as a bash/zsh function.
func posixFunction(s Shortcut) string {
	var pre []string
	if strings.Contains(s.Command, branchToken) {
		pre = append(pre, "__bdock_b=$(bdock-git-branch) || return")
	}
	if strings.Contains(s.Command, defaultToken) {
		pre = append(pre, "__bdock_d=$(bdock-git-default-branch) || return")
	}
	body := strings.NewReplacer(branchToken, `"$__bdock_b"`, defaultToken, `"$__bdock_d"`).Replace(s.Command)
	return fmt.Sprintf("function %s { local __bdock_b __bdock_d; %s; command %s \"$@\"; }\n", s.Name, strings.Join(pre, "; "), body)
}

// posixShortcuts renders every shortcut for bash or zsh.
func posixShortcuts(b *strings.Builder, shortcuts []Shortcut) {
	for _, s := range shortcuts {
		if s.Dynamic() {
			b.WriteString(posixFunction(s))
		} else {
			fmt.Fprintf(b, "alias %s='%s'\n", s.Name, s.Command)
		}
	}
}

// The function keyword keeps zsh and bash from alias-expanding a name that a
// framework (for example Oh My Zsh) already defined as an alias.
const posixHelpers = `function bdock-git-branch {
    command git symbolic-ref --quiet --short HEAD || {
        printf '%s\n' 'bdock: not on a branch (detached HEAD)' >&2
        return 1
    }
}
function bdock-git-default-branch {
    local ref
    if ref=$(command git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null); then
        printf '%s\n' "${ref#origin/}"
    elif command git show-ref --quiet --verify refs/heads/main; then
        printf '%s\n' main
    else
        printf '%s\n' master
    fi
}
`

func renderBash(shortcuts []Shortcut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", header)
	b.WriteString(`# Source from an interactive bash (for example /etc/bash.bashrc).
# Opt out: BDOCK_SHORTCUTS=0 (aliases), BDOCK_STARSHIP=0 (prompt), BDOCK_FASTFETCH=0 (startup greeting).
[ -n "${BASH_VERSION-}" ] || return 0
case $- in *i*) ;; *) return 0 ;; esac

if [ "${BDOCK_FASTFETCH-1}" != 0 ] && [ "${__bdock_fastfetch_pid-}" != "$$" ] && command -v fastfetch >/dev/null 2>&1; then
    __bdock_fastfetch_pid=$$
    command fastfetch || :
fi

if [ "${BDOCK_STARSHIP-1}" != 0 ] && [ -z "${STARSHIP_SHELL-}" ] && command -v starship >/dev/null 2>&1; then
    eval "$(starship init bash)"
fi

[ "${BDOCK_SHORTCUTS-1}" != 0 ] || return 0

`)
	b.WriteString(posixHelpers)
	b.WriteString("\n")
	posixShortcuts(&b, shortcuts)
	renderNavigation(&b, "bash")
	return b.String()
}

func renderZsh(shortcuts []Shortcut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", header)
	b.WriteString(`# Source from an interactive zsh (for example /etc/zshrc or /etc/zsh/zshrc).
# Opt out: BDOCK_SHORTCUTS=0 (aliases), BDOCK_STARSHIP=0 (prompt),
# BDOCK_ZSH_EXPAND=0 (keep aliases, but do not expand them on space),
# BDOCK_FASTFETCH=0 (startup greeting).
[[ -n ${ZSH_VERSION-} ]] || return 0
[[ -o interactive ]] || return 0

if [[ ${BDOCK_FASTFETCH-1} != 0 && ${__bdock_fastfetch_pid-} != $$ ]] && (( $+commands[fastfetch] )); then
    typeset -g __bdock_fastfetch_pid=$$
    command fastfetch || :
fi

if [[ ${BDOCK_STARSHIP-1} != 0 && -z ${STARSHIP_SHELL-} ]] && (( $+commands[starship] )); then
    eval "$(starship init zsh)"
fi

[[ ${BDOCK_SHORTCUTS-1} != 0 ]] || return 0

`)
	b.WriteString(posixHelpers)
	b.WriteString("\ntypeset -gA __bdock_shortcuts\n__bdock_shortcuts=(\n")
	for _, s := range shortcuts {
		if !s.Dynamic() {
			fmt.Fprintf(&b, "    %s '%s'\n", s.Name, s.Command)
		}
	}
	b.WriteString(")\n")
	posixShortcuts(&b, shortcuts)
	renderNavigation(&b, "zsh")
	b.WriteString(`
# Like fish abbreviations: a static shortcut typed in command position expands
# in place when space is pressed, so the real command is visible and in
# history. Branch-dependent shortcuts stay functions. Ctrl+Space inserts a
# plain space.
if [[ ${BDOCK_ZSH_EXPAND-1} != 0 ]]; then
    __bdock-expand-space() {
        emulate -L zsh
        setopt extendedglob
        local word=${LBUFFER##*[[:space:];|&]}
        local before=${LBUFFER%$word}
        if [[ -n $word && -n ${__bdock_shortcuts[$word]-} ]] &&
            [[ $before == [[:space:]]# || $before == *[\;\|\&][[:space:]]# ]]; then
            LBUFFER=$before${__bdock_shortcuts[$word]}
        fi
        zle self-insert
    }
    zle -N __bdock-expand-space
    bindkey -M emacs ' ' __bdock-expand-space
    bindkey -M viins ' ' __bdock-expand-space
    bindkey -M emacs '^ ' magic-space
    bindkey -M viins '^ ' magic-space
    bindkey -M isearch ' ' self-insert
fi
`)
	return b.String()
}

func renderFish(shortcuts []Shortcut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", header)
	b.WriteString(`# Loaded automatically when <toolbox>/share is in XDG_DATA_DIRS.
# Opt out: BDOCK_SHORTCUTS=0 (abbreviations), BDOCK_STARSHIP=0 (prompt), BDOCK_FASTFETCH=0 (startup greeting).
status is-interactive; or return 0

if test "$BDOCK_FASTFETCH" != 0; and test "$__bdock_fastfetch_pid" != "$fish_pid"; and command -q fastfetch
    set -g __bdock_fastfetch_pid $fish_pid
    command fastfetch; or true
end

if test "$BDOCK_STARSHIP" != 0; and not set -q STARSHIP_SHELL; and command -q starship
    starship init fish | source
end

test "$BDOCK_SHORTCUTS" != 0; or return 0

function bdock-git-branch -d 'Current Git branch; fails on a detached HEAD'
    command git symbolic-ref --quiet --short HEAD; and return 0
    echo 'bdock: not on a branch (detached HEAD)' >&2
    return 1
end

function bdock-git-default-branch -d 'origin/HEAD, else main, else master'
    if set -l ref (command git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null)
        string replace -r '^origin/' '' -- $ref
    else if command git show-ref --quiet --verify refs/heads/main
        echo main
    else
        echo master
    end
end

`)
	for _, s := range shortcuts {
		if !s.Dynamic() {
			fmt.Fprintf(&b, "abbr --add --global -- %s '%s'\n", s.Name, s.Command)
			continue
		}
		words := s.Words()
		fmt.Fprintf(&b, "function %s --wraps '%s %s'\n", s.Name, words[0], words[1])
		if strings.Contains(s.Command, branchToken) {
			b.WriteString("    set -l __bdock_b (bdock-git-branch); or return 1\n")
		}
		if strings.Contains(s.Command, defaultToken) {
			b.WriteString("    set -l __bdock_d (bdock-git-default-branch); or return 1\n")
		}
		body := strings.NewReplacer(branchToken, "$__bdock_b", defaultToken, "$__bdock_d").Replace(s.Command)
		fmt.Fprintf(&b, "    command %s $argv\nend\n", body)
	}
	renderNavigation(&b, "fish")
	return b.String()
}

func renderNu(shortcuts []Shortcut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", header)
	b.WriteString(`# Loaded automatically by interactive Nushell when <toolbox>/share is in
# XDG_DATA_DIRS. Opt out with BDOCK_STARSHIP=0 (prompt) or
# BDOCK_FASTFETCH=0 (startup greeting). Nushell
# definitions cannot be conditional, so the shortcuts are always defined;
# leave <toolbox>/share out of XDG_DATA_DIRS to skip this file entirely.

if $nu.is-interactive and ($env.BDOCK_FASTFETCH? | default "1") != "0" and ($env.__bdock_fastfetch_pid? | default 0) != $nu.pid and (which fastfetch | is-not-empty) {
    $env.__bdock_fastfetch_pid = $nu.pid
    try { ^fastfetch } catch { null }
}

if ($env.BDOCK_STARSHIP? | default "1") != "0" and ($env.STARSHIP_SHELL? | is-empty) and (which starship | is-not-empty) {
    # Equivalent to ` + "`starship init nu`" + `, kept static because autoload files
    # cannot evaluate generated code.
    $env.STARSHIP_SHELL = "nu"
    $env.STARSHIP_SESSION_KEY = (random chars -l 16)
    $env.PROMPT_MULTILINE_INDICATOR = (^starship prompt --continuation)
    $env.PROMPT_INDICATOR = ""
    $env.PROMPT_COMMAND = {||
        let duration = if $env.CMD_DURATION_MS == "0823" { 0 } else { $env.CMD_DURATION_MS }
        ^starship prompt --cmd-duration $duration $"--status=($env.LAST_EXIT_CODE)" --terminal-width (term size).columns
    }
    $env.PROMPT_COMMAND_RIGHT = {||
        let duration = if $env.CMD_DURATION_MS == "0823" { 0 } else { $env.CMD_DURATION_MS }
        ^starship prompt --right --cmd-duration $duration $"--status=($env.LAST_EXIT_CODE)" --terminal-width (term size).columns
    }
    $env.config = ($env.config? | default {} | merge {render_right_prompt_on_last_line: true})
}

# Current Git branch; fails on a detached HEAD.
def bdock-git-branch []: nothing -> string {
    let r = (do { ^git symbolic-ref --quiet --short HEAD } | complete)
    if $r.exit_code != 0 {
        error make --unspanned {msg: "bdock: not on a branch (detached HEAD)"}
    }
    $r.stdout | str trim
}

# origin/HEAD, else main, else master.
def bdock-git-default-branch []: nothing -> string {
    let r = (do { ^git symbolic-ref --quiet --short refs/remotes/origin/HEAD } | complete)
    if $r.exit_code == 0 {
        return ($r.stdout | str trim | str replace --regex '^origin/' '')
    }
    let main = (do { ^git show-ref --quiet --verify refs/heads/main } | complete)
    if $main.exit_code == 0 { "main" } else { "master" }
}

`)
	for _, s := range shortcuts {
		if !s.Dynamic() {
			fmt.Fprintf(&b, "alias %s = ^%s\n", s.Name, s.Command)
			continue
		}
		r := strings.NewReplacer(branchToken, "(bdock-git-branch)", defaultToken, "(bdock-git-default-branch)")
		fmt.Fprintf(&b, "def --wrapped %s [...rest] { ^%s ...$rest }\n", s.Name, r.Replace(s.Command))
	}
	b.WriteString("\n# Navigation: .., ... and .... are native implicit cd in Nushell; - cannot be\n# supported (the parser treats a lone - as a string), so none is defined here.\n")
	return b.String()
}
