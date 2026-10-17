package toolconfig

// Source is the optional per-package third-party source block of the catalog
// (docs/design/third-party-source-mirror/data-model.md). It classifies the
// source obligation of the shipped binary and declares the recipe for the
// source mirror. Absent blocks are omitted from the catalog digest.
type Source struct {
	Obligation   string          `json:"obligation"`
	Licenses     []string        `json:"licenses,omitempty"`
	Basis        string          `json:"basis,omitempty"`
	Upstream     *SourceUpstream `json:"upstream,omitempty"`
	Recipe       *SourceRecipe   `json:"recipe,omitempty"`
	VersionRegex string          `json:"version_regex,omitempty"`
}

// SourceUpstream names the upstream repository and the tarball URL template.
type SourceUpstream struct {
	Repository string `json:"repository"`
	Tarball    string `json:"tarball"`
}

// SourceRecipe is the closed-vocabulary recipe for assembling a source bundle.
type SourceRecipe struct {
	Kind             string   `json:"kind"`
	Excluded         []string `json:"excluded,omitempty"`
	StaticSystemLibs []string `json:"static_system_libs,omitempty"`
	FreezeSource     string   `json:"freeze_source,omitempty"`
	FreezeFile       string   `json:"freeze_file,omitempty"`
}
