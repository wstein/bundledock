package toolbundle

import (
	"bundledock/internal/toolconfig"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const NodeProfile = "node-runtime"

var nodeRoot = regexp.MustCompile(`^node-v[0-9]+\.[0-9]+\.[0-9]+-linux-(x64|arm64)$`)
var nodeLinks = map[string]string{"payload/bin/npm": "../lib/node_modules/npm/bin/npm-cli.js", "payload/bin/npx": "../lib/node_modules/npm/bin/npx-cli.js"}

func nodeLinkTarget(root *os.Root, name, target string) error {
	if nodeLinks[name] != target || target == "" {
		return fmt.Errorf("unsupported Node link %q", name)
	}
	f, err := OpenFile(root, "payload/lib/node_modules/npm/bin/"+strings.TrimPrefix(target, "../lib/node_modules/npm/bin/"))
	if err != nil {
		return err
	}
	return f.Close()
}
func profileLinkTarget(root *os.Root, profile, name, target string) error {
	if profile == BunProfile {
		return bunLinkTarget(root, name, target)
	}
	if profile == NodeProfile {
		return nodeLinkTarget(root, name, target)
	}
	return standaloneLinkTarget(root, name, target)
}
func ValidateNodeSpec(s Spec, arch string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("invalid Node architecture")
	}
	if err := toolconfig.ValidateSpec(s); err != nil {
		return err
	}
	v := strings.TrimPrefix(s.Version, "v")
	if _, err := toolconfig.ParseVersion(v); err != nil {
		return err
	}
	a := arch
	if a == "amd64" {
		a = "x64"
	}
	if s.Version != "v"+v || s.URL != "https://nodejs.org/dist/"+s.Version+"/node-"+s.Version+"-linux-"+a+".tar.gz" || s.Verification != "publisher-sha256" {
		return fmt.Errorf("invalid official Node provenance")
	}
	return nil
}
func writeNodeLaunchers(root *os.Root) error {
	for _, name := range []string{"node", "npm", "npx"} {
		command := "exec \"$root/payload/bin/node\" \"$@\""
		if name != "node" {
			command = "export PATH=\"$root/bin:$PATH\"\n" + "exec \"$root/payload/bin/node\" \"$root/payload/lib/node_modules/npm/bin/" + name + "-cli.js\" \"$@\""
		}
		script := "#!/bin/sh\nset -eu\nroot=$(CDPATH= cd -- \"${0%/*}/..\" && pwd)\n" + command + "\n"
		if err := WriteFile(root, "bin/"+name, strings.NewReader(script), 0755); err != nil {
			return err
		}
	}
	return nil
}

// WriteNodeManifest binds the narrow Node graph to official native provenance.
func WriteNodeManifest(root *os.Root, arch string, tools map[string]Spec) error {
	return writePayloadManifest(root, arch, tools, NodeProfile)
}
