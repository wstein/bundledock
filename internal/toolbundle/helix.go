package toolbundle

import (
	"os"
	"regexp"
	"strings"
)

// Helix stable distributions contain regular runtime files and compiled grammars.
// The profile relocates the whole tree without admitting links or compiling tools.
var helixArchiveRoot = regexp.MustCompile(`^helix-[1-9][0-9]*\.(0?[1-9]|1[0-2])(?:\.(0|[1-9][0-9]*))?-(x86_64|aarch64)-linux$`)

func writeHelixLauncher(root *os.Root) error {
	for _, name := range []string{"lib/helix/runtime/grammars", "lib/helix/runtime/queries", "lib/helix/runtime/themes"} {
		if err := CheckPath(root, name); err != nil {
			return err
		}
		info, err := root.Stat(name)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return &os.PathError{Op: "validate Helix runtime", Path: name, Err: os.ErrInvalid}
		}
	}
	return WriteFile(root, "bin/hx", strings.NewReader("#!/bin/sh\nset -eu\nbundle=$(CDPATH= cd -- \"${0%/*}/..\" && pwd)\nexport HELIX_RUNTIME=\"$bundle/lib/helix/runtime\"\nexec \"$bundle/lib/helix/hx\" \"$@\"\n"), 0755)
}
