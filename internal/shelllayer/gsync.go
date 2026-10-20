package shelllayer

import (
	_ "embed"
	"strings"
)

// gsync and gsync-all resync a clone after GitHub rebase-merges rewrote SHAs.
// They are not table rows, because a row is one plain command: the logic (work
// tree and in-progress checks, detached HEAD, one status line per submodule)
// lives once in gsync.sh, a POSIX sh script that every shell runs through a
// small function, so all four shells behave the same.

//go:embed gsync.sh
var gsyncScript string

// GsyncNames are the shell names the layer defines for the script, in
// documentation order. They are fixed: validName still rejects a dash, and the
// table parser rejects a name that is not a plain command.
var GsyncNames = []string{"gsync", "gsync-all"}

// GsyncScript returns the embedded sh script.
func GsyncScript() string { return gsyncScript }

const gsyncHeading = "\n# gsync and gsync-all (gsync.sh): resync after GitHub rebase-merges rewrote SHAs.\n"

func gsyncPosix() string {
	return gsyncHeading +
		"function bdock-gsync-sh { command sh -c " + posixQuote(gsyncScript) + " bdock-gsync \"$@\"; }\n" +
		"function gsync { bdock-gsync-sh sync \"$@\"; }\n" +
		"function gsync-all { bdock-gsync-sh all; }\n"
}

func gsyncFish() string {
	return gsyncHeading +
		"function bdock-gsync-sh\n    command sh -c " + fishQuote(gsyncScript) + " bdock-gsync $argv\nend\n" +
		"function gsync -d 'fetch --prune --all, then pull --rebase --autostash'\n    bdock-gsync-sh sync $argv\nend\n" +
		"function gsync-all -d 'one status line per repository and submodule; fetch --prune only'\n    bdock-gsync-sh all\nend\n"
}

func gsyncNu() string {
	return gsyncHeading +
		"def --wrapped bdock-gsync-sh [...rest] { ^sh -c r#'\n" + strings.TrimSuffix(gsyncScript, "\n") + "\n'# bdock-gsync ...$rest }\n" +
		"def --wrapped gsync [...rest] { bdock-gsync-sh sync ...$rest }\n" +
		"def gsync-all [] { bdock-gsync-sh all }\n"
}
