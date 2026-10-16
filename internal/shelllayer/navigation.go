package shelllayer

import (
	"fmt"
	"regexp"
	"strings"
)

// Navigation names (.., ..., ...., -) are not shortcut names: validName
// rejects them, and the table parser rejects `cd` and path words, so none of
// that was loosened. They live here as a short fixed list with their own
// strict patterns and explicit per-shell rendering. Whether a name is
// generated depends on the shell: a shell that already runs the name keeps its
// own behavior, and a shell that cannot support it gets nothing.

// NavState says how a shell treats a navigation name.
type NavState string

const (
	// NavNative: the shell runs the name by itself; the layer adds nothing.
	NavNative NavState = "native"
	// NavLayer: the shell does not know the name; the layer defines it.
	NavLayer NavState = "layer"
	// NavUnsupported: the shell cannot run the name even when defined; the
	// layer leaves it undefined rather than add a definition that never runs.
	NavUnsupported NavState = "unsupported"
)

var (
	// validNavName admits exactly the names ".." "..." "...." and "-".
	validNavName = regexp.MustCompile(`^(\.\.|\.\.\.|\.\.\.\.|-)$`)
	// validNavCommand admits exactly `cd -` and `cd ..`, `cd ../..`, ... : the
	// only words are cd and a dash or a chain of ".." segments.
	validNavCommand = regexp.MustCompile(`^cd (-|\.\.(/\.\.)*)$`)
)

// navigation is the fixed navigation table, in documentation order.
var navigation = []Shortcut{
	{"..", "cd .."},
	{"...", "cd ../.."},
	{"....", "cd ../../.."},
	{"-", "cd -"},
}

// navigationSupport records what each shell does with each name, measured in
// an interactive pseudo-terminal with bash 5.2.37, zsh 5.9, fish 4.9.3 and
// Nushell 0.116.1 (TestNavigationShortcutsChangeDirectoryInEveryShell re-checks
// it, and docs/toolbox.md carries the same table):
//
//   - bash: none of the names is native; aliases work (`alias -- -=...`).
//   - zsh: `..`, `...` and `....` are aliases; `-` is a built-in precommand
//     modifier (`- cmd`), and an alias would replace it, so it is left alone.
//   - fish: `..` is native; `...`, `....` and `-` work as abbreviations.
//   - Nushell: `..`, `...` and `....` are native implicit cd (only for a bare
//     path typed alone in the REPL); `-` cannot be supported: `alias - = ...`
//     and `def "-"` are accepted but never run, because the parser treats a
//     lone `-` as a string.
var navigationSupport = map[string]map[string]NavState{
	"bash": {"..": NavLayer, "...": NavLayer, "....": NavLayer, "-": NavLayer},
	"zsh":  {"..": NavLayer, "...": NavLayer, "....": NavLayer, "-": NavUnsupported},
	"fish": {"..": NavNative, "...": NavLayer, "....": NavLayer, "-": NavLayer},
	"nu":   {"..": NavNative, "...": NavNative, "....": NavNative, "-": NavUnsupported},
}

// Navigation returns the navigation shortcuts. It panics if the fixed list
// ever stops matching its patterns.
func Navigation() []Shortcut {
	out := make([]Shortcut, len(navigation))
	for i, s := range navigation {
		if !validNavName.MatchString(s.Name) || !validNavCommand.MatchString(s.Command) {
			panic(fmt.Sprintf("shelllayer: invalid navigation entry %q %q", s.Name, s.Command))
		}
		out[i] = s
	}
	return out
}

// NavigationSupport reports how shell ("bash", "zsh", "fish" or "nu") treats
// the navigation name; an unknown shell or name is unsupported.
func NavigationSupport(shell, name string) NavState {
	if state, ok := navigationSupport[shell][name]; ok {
		return state
	}
	return NavUnsupported
}

// navigationLayer returns the navigation shortcuts the layer defines for shell.
func navigationLayer(shell string) []Shortcut {
	var out []Shortcut
	for _, s := range Navigation() {
		if NavigationSupport(shell, s.Name) == NavLayer {
			out = append(out, s)
		}
	}
	return out
}

// renderNavigation writes the definitions for shell: aliases for bash and zsh
// (`--` ends option parsing so the name `-` is accepted), abbreviations for
// fish, nothing for Nushell, whose names are native or unsupported.
func renderNavigation(b *strings.Builder, shell string) {
	defs := navigationLayer(shell)
	if len(defs) == 0 {
		return
	}
	b.WriteString("\n# Navigation names this shell does not run natively.\n")
	for _, s := range defs {
		switch shell {
		case "bash", "zsh":
			fmt.Fprintf(b, "alias -- %s='%s'\n", s.Name, s.Command)
		case "fish":
			fmt.Fprintf(b, "abbr --add --global -- %s '%s'\n", s.Name, s.Command)
		default:
			panic("shelllayer: no navigation rendering for " + shell)
		}
	}
}
