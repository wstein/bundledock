package toolbundle

import (
	_ "embed"
	"os"
	"strings"
)

//go:embed specify.sh
var specifyLauncher string

// The owned launcher uses the existing uv payload. Spec Kit is not installed in
// the immutable bundle and is deliberately absent from the artifact lock.
func writeSpecifyLauncher(root *os.Root) error {
	return WriteFile(root, "bin/specify", strings.NewReader(specifyLauncher), 0755)
}
