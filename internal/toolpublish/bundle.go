package toolpublish

import (
	"fmt"
	"strings"
)

// Bundle is the bounded publication source selection; toolbox remains default.
type Bundle struct{ Name, Directory, Workflow, Artifact string }

func SelectBundle(name string) (Bundle, error) {
	switch name {
	case "", "toolbox":
		return Bundle{"toolbox", "bundles/toolbox", "bundledock-toolbox.yml", "resolved-tools"}, nil
	case "node":
		return Bundle{"node", "bundles/node", "bundledock-node.yml", "resolved-node"}, nil
	case "code-server":
		return Bundle{"code-server", "bundles/code-server", "bundledock-code-server.yml", "resolved-code-server"}, nil
	default:
		return Bundle{}, fmt.Errorf("bundle must be toolbox, code-server or node")
	}
}
func RepositoryImageForBundle(prefix, name string) (string, error) {
	b, err := SelectBundle(name)
	if err != nil {
		return "", err
	}
	image := strings.TrimSuffix(prefix, "/") + "/" + b.Name
	if !imagePattern.MatchString(image) {
		return "", fmt.Errorf("REPOSITORY_URL must be quay.io/<namespace> with an optional trailing slash")
	}
	return image, nil
}
func bundleForImage(image string) Bundle {
	name := "toolbox"
	if strings.HasSuffix(image, "/node") {
		name = "node"
	}
	if strings.HasSuffix(image, "/code-server") {
		name = "code-server"
	}
	b, _ := SelectBundle(name)
	return b
}

// BundleForMetadata selects source paths from the already validated image identity.
func BundleForMetadata(m Metadata) Bundle { return bundleForImage(m.Image) }
