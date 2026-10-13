package toolbundle

import (
	_ "embed"
	"os"
)

//go:embed specify.sh
var specifyLauncher string

//go:embed python/specify/specify.requirements.txt
var specifyRequirements string

// The immutable toolbox carries owned source and metadata, never a Python
// runtime or installed package. Plain pip installs into writable tool state.
func writeSpecifyLauncher(root *os.Root) error {
	return writePythonLauncher(root, "specify", specifyLauncher, specifyRequirements)
}

func validateSpecifyLauncher(root *os.Root) error {
	return validatePythonLauncher(root, "specify", specifyLauncher, specifyRequirements)
}
