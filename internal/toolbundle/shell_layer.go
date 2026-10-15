package toolbundle

import (
	"bytes"
	"os"
	"sort"

	"bundledock/internal/shelllayer"
	"bundledock/internal/toolconfig"
)

// hasShellLayer reports whether a build carries the shared shell layer: the
// toolbox, recognized by its starship prompt package. Single-tool carriers
// (Node, Bun, code-server, agent CLIs, OpenTofu) never do.
func hasShellLayer(packages map[string]toolconfig.Package) bool {
	_, ok := packages["starship"]
	return ok
}

// writeShellLayer copies BundleDock's own generated shell init files, the
// shortcut table and the retained plugin-git notice into the bundle. It runs
// no shell and executes nothing; the files are committed, reviewed output of
// internal/shelllayer.
func writeShellLayer(root *os.Root) error {
	files, err := shelllayer.Files()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := WriteFile(root, name, bytes.NewReader(files[name]), 0644); err != nil {
			return err
		}
	}
	return nil
}
