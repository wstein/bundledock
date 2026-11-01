package toolpublish

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	normalizeWord = regexp.MustCompile(`(?:\S*/)?[\w-]+ normalize-mtimes -root ([^\s;&|"']+)`)
	shellComment  = regexp.MustCompile(`(^|\s)#.*$`)
	fromLine      = regexp.MustCompile(`(?i)^FROM\s+(?:--\S+\s+)*\S+(?:\s+AS\s+(\S+))?`)
)

type dockerStage struct {
	name string
	text string // active instructions only, continuation lines joined, newline separated
}

// parseStages splits a Dockerfile into stages and drops comment lines, so a
// commented-out instruction does not count. Backslash continuations are joined.
func parseStages(dockerfile string) []dockerStage {
	var stages []dockerStage
	var cur *dockerStage
	var pending string
	for _, line := range strings.Split(dockerfile, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.HasSuffix(line, "\\") {
			pending += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		line = pending + line
		pending = ""
		// In the joined instruction a shell comment runs to the end of it.
		line = shellComment.ReplaceAllString(line, "$1")
		if m := fromLine.FindStringSubmatch(line); m != nil {
			stages = append(stages, dockerStage{name: m[1]})
			cur = &stages[len(stages)-1]
		}
		if cur != nil && strings.TrimSpace(line) != "" {
			cur.text += line + "\n"
		}
	}
	return stages
}

// checkNormalizedBeforeCopy returns one problem per root that the final stage
// copies without a normalize-mtimes step that comes after every other mention
// of the normalized root (the writes) in its source stage. Position is the text
// order of the active instructions.
func checkNormalizedBeforeCopy(dockerfile string) []string {
	stages := parseStages(dockerfile)
	if len(stages) < 2 {
		return []string{"no multi-stage Dockerfile"}
	}
	byName := map[string]dockerStage{}
	for _, st := range stages {
		byName[st.name] = st
	}
	var problems []string
	copies := 0
	for _, line := range strings.Split(stages[len(stages)-1].text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || !strings.EqualFold(f[0], "COPY") {
			continue
		}
		copies++
		if len(f) != 4 || !strings.HasPrefix(f[1], "--from=") || !strings.HasSuffix(f[2], "/") || f[3] != "/" {
			problems = append(problems, fmt.Sprintf("final COPY must copy a whole staged root to /: %q", line))
			continue
		}
		root := strings.TrimSuffix(f[2], "/")
		st, ok := byName[strings.TrimPrefix(f[1], "--from=")]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: unknown source stage %s", root, f[1]))
			continue
		}
		problems = append(problems, checkStage(st, root)...)
	}
	if copies == 0 {
		problems = append(problems, "final stage has no COPY")
	}
	return problems
}

func checkStage(st dockerStage, root string) []string {
	// Blank the normalize commands (keeping offsets) and remember the last one
	// whose root covers the copied root.
	last := -1
	covering := ""
	blanked := st.text
	for _, c := range normalizeCalls(st.text) {
		blanked = blanked[:c.start] + strings.Repeat(" ", c.end-c.start) + blanked[c.end:]
		if root == c.root || strings.HasPrefix(root, c.root+"/") {
			last, covering = c.start, c.root
		}
	}
	if last < 0 {
		return []string{root + " is copied but never passed to normalize-mtimes"}
	}
	mention := regexp.MustCompile(regexp.QuoteMeta(covering) + `($|[^A-Za-z0-9_.-])`)
	var problems []string
	for _, w := range mention.FindAllStringIndex(blanked, -1) {
		if w[0] > last {
			problems = append(problems, fmt.Sprintf("%s is written (%q) after its normalize-mtimes step", covering, strings.TrimSpace(blanked[w[0]:min(len(blanked), w[0]+40)])))
		}
	}
	return problems
}

type normalizeCall struct {
	start, end int
	root       string
}

// normalizeCalls finds normalize-mtimes calls that run as a command: the text
// before is the start of a RUN or && or ;, and the text after is && or ; or the
// end of the instruction. echo, another command word, "|| true" and pipes do not
// qualify. It cannot see conditionals, subshells, variables or globs.
func normalizeCalls(text string) []normalizeCall {
	var calls []normalizeCall
	for _, m := range normalizeWord.FindAllStringSubmatchIndex(text, -1) {
		before := strings.TrimRight(text[:m[0]], " \t")
		after := strings.TrimLeft(text[m[1]:], " \t")
		lineStart := strings.LastIndex(before, "\n") + 1
		okBefore := strings.HasSuffix(before, "&&") || strings.HasSuffix(before, ";") ||
			regexp.MustCompile(`(?i)^RUN$`).MatchString(strings.TrimSpace(before[lineStart:]))
		okAfter := after == "" || strings.HasPrefix(after, "\n") || strings.HasPrefix(after, "&&") || strings.HasPrefix(after, ";")
		if okBefore && okAfter {
			calls = append(calls, normalizeCall{m[0], m[1], text[m[2]:m[3]]})
		}
	}
	return calls
}
