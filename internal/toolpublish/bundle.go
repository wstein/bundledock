package toolpublish

import (
	"fmt"
	"strings"
)

// Bundle is the bounded publication source selection; toolbox remains default.
type Bundle struct{ Name, Directory, Workflow, Artifact, Repository, Aliases string }

// CanonicalBundles lists all official bundle names in fixed order.
var CanonicalBundles = []string{"toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu"}

func SelectBundle(name string) (Bundle, error) {
	if name == "" {
		name = "toolbox"
	}
	switch name {
	case "toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu":
		repository, aliases, artifact := name, "stable latest", "resolved-"+name
		if name == "toolbox" {
			artifact = "resolved-tools"
		}
		if name == "node" {
			aliases = "stable lts latest"
		}
		if name == "node-current" {
			repository, aliases = "node", "current"
		}
		return Bundle{name, "bundles/" + name, "publish-bundles.yml", artifact, repository, aliases}, nil
	default:
		return Bundle{}, fmt.Errorf("unknown canonical publication bundle %q", name)
	}

}
func RepositoryImageForBundle(prefix, name string) (string, error) {
	b, err := SelectBundle(name)
	if err != nil {
		return "", err
	}
	image := strings.TrimSuffix(prefix, "/") + "/" + b.Repository
	if !imagePattern.MatchString(image) {
		return "", fmt.Errorf("REPOSITORY_URL must be quay.io/<namespace> with an optional trailing slash")
	}
	return image, nil
}
func bundleForImage(image string) Bundle {
	name := image[strings.LastIndex(image, "/")+1:]
	b, _ := SelectBundle(name)
	return b
}

// BundleForMetadata selects source paths from the already validated image identity and explicit canonical source.
func BundleForMetadata(m Metadata) Bundle {
	if m.Bundle != "" {
		b, _ := SelectBundle(m.Bundle)
		return b
	}
	return bundleForImage(m.Image)
}

// SourceTagNamespaces reserves revisions across the fixed shared Node repository.
func (b Bundle) SourceTagNamespaces() []string {
	if b.Repository == "node" {
		return []string{"node", "node-current"}
	}
	return []string{b.Name}
}
func (b Bundle) PublicationAliases() []string { return strings.Fields(b.Aliases) }
func (b Bundle) BaselineAlias() string {
	if b.Name == "node-current" {
		return "current"
	}
	return "stable"
}
func validateBundleImage(b Bundle, image string) error {
	if b.Name == "" || !imagePattern.MatchString(image) || !strings.HasSuffix(image, "/"+b.Repository) {
		return fmt.Errorf("publication source bundle and output repository differ")
	}
	return nil
}

// SourceTagReservations collects immutable versions from canonical source namespaces.
func SourceTagReservations(b Bundle, refs []byte) ([]string, error) {
	var tags []string
	for _, line := range strings.Split(string(refs), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid source tag listing")
		}
		if strings.HasSuffix(fields[1], "^{}") {
			continue
		}
		for _, name := range b.SourceTagNamespaces() {
			if tag, ok := strings.CutPrefix(fields[1], "refs/tags/"+name+"/"); ok {
				tags = append(tags, tag)
			}
		}
	}
	return tags, nil
}
