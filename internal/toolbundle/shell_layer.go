package toolbundle

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"

	"bundledock/internal/toolconfig"
)

// BuildOption adjusts a bundle build.
type BuildOption func(*buildOptions)

type buildOptions struct {
	shellLayer fs.FS
}

// WithShellLayer supplies the shared shell layer's source directory
// (internal/shelllayer: generated/, notices/ and shortcuts.tsv). Only the
// toolbox takes it. The builder reads these committed files at build time and
// never imports the layer, so no other bundle, builder or volume initializer
// carries any part of it.
func WithShellLayer(source fs.FS) BuildOption {
	return func(o *buildOptions) { o.shellLayer = source }
}

// shellLayerFiles maps the layer's source files to their place in the toolbox.
// fish and Nushell load theirs from <toolbox>/share through XDG_DATA_DIRS; bash
// and zsh source theirs from the consumer's system rc file.
var shellLayerFiles = map[string]string{
	"generated/init.bash":        "share/shell/init.bash",
	"generated/init.zsh":         "share/shell/init.zsh",
	"generated/bdock-shell.fish": "share/fish/vendor_conf.d/bdock-shell.fish",
	"generated/bdock-shell.nu":   "share/nushell/vendor/autoload/bdock-shell.nu",
	"shortcuts.tsv":              "share/shell/shortcuts.tsv",
	"notices/plugin-git-LICENSE": "licenses/shell-layer/plugin-git-LICENSE",
	"notices/SOURCE":             "licenses/shell-layer/SOURCE",
}

const maxShellLayerFile = 1 << 20

// isToolbox reports whether a build is the toolbox, recognized by its starship
// prompt package. Single-tool carriers (Node, Bun, code-server, agent CLIs,
// OpenTofu) are not.
func isToolbox(packages map[string]toolconfig.Package) bool {
	_, ok := packages["starship"]
	return ok
}

// shellLayerStep decides the layer for one build: the toolbox requires it and
// every other bundle refuses it.
func shellLayerStep(root *os.Root, packages map[string]toolconfig.Package, o buildOptions) error {
	switch {
	case isToolbox(packages) && o.shellLayer == nil:
		return fmt.Errorf("the toolbox requires the shared shell layer source (tools-bundle --shell-layer)")
	case !isToolbox(packages) && o.shellLayer != nil:
		return fmt.Errorf("the shared shell layer belongs to the toolbox only")
	case o.shellLayer == nil:
		return nil
	}
	return writeShellLayer(root, o.shellLayer)
}

// writeShellLayer copies exactly the expected regular files of the layer
// source into the bundle. It executes nothing.
func writeShellLayer(root *os.Root, source fs.FS) error {
	names := make([]string, 0, len(shellLayerFiles))
	for name := range shellLayerFiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		info, err := fs.Stat(source, name)
		if err != nil {
			return fmt.Errorf("shell layer source %s: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Size() > maxShellLayerFile {
			return fmt.Errorf("shell layer source %s must be a regular file of at most %d bytes", name, maxShellLayerFile)
		}
		f, err := source.Open(name)
		if err != nil {
			return err
		}
		err = WriteFile(root, shellLayerFiles[name], io.LimitReader(f, maxShellLayerFile), 0644)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
