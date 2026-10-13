package toolbundle

import (
	"embed"
	"os"
	"strings"
)

//go:embed tldr.sh
var tldrLauncher string

// The base distribution already supplies tldr:cli. Its optional "cli" extra
// contains development tools, not dependencies needed to run the command.
//
//go:embed python/tldr/tldr.requirements.txt
var tldrRequirements string

//go:embed python/tldr/*.LICENSE python/tldr/SOURCE
var tldrNotices embed.FS

func writeTldrLauncher(root *os.Root) error {
	if err := writePythonLauncher(root, "tldr", tldrLauncher, tldrRequirements); err != nil {
		return err
	}
	for _, name := range []string{"tldr.LICENSE", "shtab.LICENSE", "termcolor.LICENSE", "SOURCE"} {
		data, err := tldrNotices.ReadFile("python/tldr/" + name)
		if err != nil {
			return err
		}
		if err := WriteFile(root, "licenses/tldr/"+name, strings.NewReader(string(data)), 0644); err != nil {
			return err
		}
	}
	return nil
}

func validateTldrLauncher(root *os.Root) error {
	return validatePythonLauncher(root, "tldr", tldrLauncher, tldrRequirements)
}
