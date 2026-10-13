package toolbundle

import (
	_ "embed"
	"os"
)

//go:embed tldr.sh
var tldrLauncher string

// The base distribution already supplies tldr:cli. Its optional "cli" extra
// contains development tools, not dependencies needed to run the command.
//
//go:embed python/tldr/tldr.requirements.txt
var tldrRequirements string

func writeTldrLauncher(root *os.Root) error {
	return writePythonLauncher(root, "tldr", tldrLauncher, tldrRequirements)
}

func validateTldrLauncher(root *os.Root) error {
	return validatePythonLauncher(root, "tldr", tldrLauncher, tldrRequirements)
}
