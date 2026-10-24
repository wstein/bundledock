package toolconfig

import (
	"fmt"
	"regexp"
	"strings"
)

// The source-build vocabulary is closed: every name below is reviewed Go code,
// not catalog data (docs/design/generic-tool-builds). A new kind, toolchain,
// define, flag or library needs its own design and review.

// VersionSource says where a source-build tool's current version and download
// path are read from. The resolver itself is a later work package.
type VersionSource struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// Build is the recipe of a provider "source-build" package.
type Build struct {
	Kind          string   `json:"kind"`
	Toolchain     string   `json:"toolchain"`
	ArchivePrefix string   `json:"archive_prefix"`
	Sources       []string `json:"sources"`
	Defines       []string `json:"defines,omitempty"`
	Cflags        []string `json:"cflags,omitempty"`
	Libs          []string `json:"libs,omitempty"`
	Static        bool     `json:"static"`
	OutputForm    string   `json:"output_form"`
	Output        string   `json:"output"`
}

var (
	// Names that reach the compiler command line must never read as options.
	sourceFileName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*\.c$`)
	outputName     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*$`)
	// toolchainHosts maps each known toolchain name to the hosts its lock URLs may use.
	toolchainHosts = map[string][]string{"zig-cc": {"ziglang.org"}}
	versionSources = map[string]bool{"sqlite-product-data": true}
	buildKinds     = map[string]bool{"c-amalgamation": true}
	outputForms    = map[string]bool{"exec": true, "static-pie": true}
	allowedDefines = map[string]bool{
		"HAVE_LINENOISE=1": true, "SQLITE_THREADSAFE=1": true, "SQLITE_OMIT_LOAD_EXTENSION=1": true,
		"SQLITE_ENABLE_FTS5=1": true, "SQLITE_ENABLE_RTREE=1": true, "SQLITE_ENABLE_DBSTAT_VTAB=1": true,
		"SQLITE_ENABLE_MATH_FUNCTIONS=1": true,
	}
	allowedCflags = map[string]bool{"-Os": true, "-O2": true}
	allowedLibs   = map[string]bool{"m": true, "pthread": true}
	// sourceBuildClasses are the publisher-data classes allowed for a main archive.
	sourceBuildClasses = map[string]bool{"publisher-sha256": true, "publisher-sha512": true, "publisher-sha3-256": true}
)

func validateSourceBuild(name string, p Package) error {
	if p.Provider != "source-build" {
		if p.Build != nil || p.VersionSource != nil {
			return fmt.Errorf("%s: the build and version_source blocks are only valid for the source-build provider", name)
		}
		if p.Install.Kind == "build-output" {
			return fmt.Errorf("%s: install kind build-output is only valid for the source-build provider", name)
		}
		return nil
	}
	if p.Build == nil {
		return fmt.Errorf("%s: source-build requires a build block", name)
	}
	b := p.Build
	if p.Install.Kind != "build-output" {
		return fmt.Errorf("%s: source-build requires install kind build-output", name)
	}
	if p.Install.Profile != "" || p.Install.Preserve || len(p.Install.Aliases) != 0 || len(p.Install.ExcludeDirectories) != 0 || len(p.Install.ArchiveNotices) != 0 || len(p.Install.ArchiveNoticeTrees) != 0 {
		return fmt.Errorf("%s: source-build allows no install profile, preserve, alias, exclusion or archive notice", name)
	}
	if p.Normalization != "semver" || p.Repository != "" || p.TagPrefix != "" || p.MetadataURL != "" || p.Sidecar != "" || p.Channel != "" {
		return fmt.Errorf("%s: source-build allows only semver normalization and no repository, tag prefix, metadata URL, sidecar or channel", name)
	}
	if p.BaseURL == "" {
		return fmt.Errorf("%s: source-build requires base_url", name)
	}
	if p.VersionSource == nil || !versionSources[p.VersionSource.Kind] {
		return fmt.Errorf("%s: source-build requires a known version_source kind", name)
	}
	if err := ValidateURL(p.VersionSource.URL, p.AllowedHosts); err != nil {
		return fmt.Errorf("%s: version_source: %w", name, err)
	}
	if !buildKinds[b.Kind] {
		return fmt.Errorf("%s: unknown build kind %q", name, b.Kind)
	}
	if _, ok := toolchainHosts[b.Toolchain]; !ok {
		return fmt.Errorf("%s: unknown toolchain %q", name, b.Toolchain)
	}
	if !b.Static {
		return fmt.Errorf("%s: build.static must be true", name)
	}
	if !outputForms[b.OutputForm] {
		return fmt.Errorf("%s: build.output_form must be exec or static-pie, not %q", name, b.OutputForm)
	}
	prefix := strings.ReplaceAll(b.ArchivePrefix, "{numeric}", "1234567")
	if !strings.HasSuffix(prefix, "/") || !validPath(strings.TrimSuffix(prefix, "/")) || strings.ContainsAny(prefix, "{}") {
		return fmt.Errorf("%s: invalid build.archive_prefix %q", name, b.ArchivePrefix)
	}
	if len(b.Sources) == 0 {
		return fmt.Errorf("%s: build.sources is empty", name)
	}
	seen := map[string]bool{}
	for _, s := range b.Sources {
		if !sourceFileName.MatchString(s) || seen[s] {
			return fmt.Errorf("%s: invalid or duplicate build source %q", name, s)
		}
		seen[s] = true
	}
	for _, d := range b.Defines {
		if !allowedDefines[d] {
			return fmt.Errorf("%s: build define %q is not allowed", name, d)
		}
	}
	for _, f := range b.Cflags {
		if !allowedCflags[f] {
			return fmt.Errorf("%s: build cflag %q is not allowed", name, f)
		}
	}
	for _, l := range b.Libs {
		if !allowedLibs[l] {
			return fmt.Errorf("%s: build lib %q is not allowed", name, l)
		}
	}
	if !outputName.MatchString(b.Output) {
		return fmt.Errorf("%s: invalid build.output %q", name, b.Output)
	}
	if _, ok := p.Install.Members[b.Output]; !ok || len(p.Install.Members) != 1 {
		return fmt.Errorf("%s: the single install member must be the build output %q", name, b.Output)
	}
	return nil
}

// validateSourceBuildSpec checks one architecture's lock entry of a package.
func validateSourceBuildSpec(name, arch string, p Package, s Spec) error {
	if p.Provider != "source-build" {
		if s.Verification == "publisher-sha3-256" {
			return fmt.Errorf("%s/%s: publisher-sha3-256 is only valid for source-build", arch, name)
		}
		if s.Toolchain != "" {
			return fmt.Errorf("%s/%s: a toolchain is only valid for source-build", arch, name)
		}
		return nil
	}
	if !sourceBuildClasses[s.Verification] {
		return fmt.Errorf("%s/%s: the source archive needs a publisher checksum class, not %q", arch, name, s.Verification)
	}
	if s.Toolchain != p.Build.Toolchain {
		return fmt.Errorf("%s/%s: lock toolchain %q differs from the catalog toolchain %q", arch, name, s.Toolchain, p.Build.Toolchain)
	}
	return nil
}

// validateToolchains checks lock.toolchains: only known toolchains used by a
// source-build package, each pinned for both architectures by one version and a
// publisher checksum.
func (c Config) validateToolchains() error {
	used := map[string]bool{}
	for name, p := range c.Catalog.Packages {
		if p.Provider != "source-build" || p.Build == nil {
			continue
		}
		used[p.Build.Toolchain] = true
		if len(c.Lock.Toolchains[p.Build.Toolchain]) != 2 {
			return fmt.Errorf("%s: toolchain %q is not pinned in the lock for both architectures", name, p.Build.Toolchain)
		}
	}
	for tc, arches := range c.Lock.Toolchains {
		hosts, known := toolchainHosts[tc]
		if !known {
			return fmt.Errorf("unknown toolchain %q in the lock", tc)
		}
		if !used[tc] {
			return fmt.Errorf("toolchain %q in the lock is used by no source-build package", tc)
		}
		version := ""
		for _, arch := range []string{"amd64", "arm64"} {
			s, ok := arches[arch]
			if !ok {
				return fmt.Errorf("toolchain %q is not pinned for exactly amd64 and arm64", tc)
			}
			if err := ValidateSpec(s); err != nil {
				return fmt.Errorf("toolchain %s/%s: %w", tc, arch, err)
			}
			if s.Verification != "publisher-sha256" && s.Verification != "publisher-sha512" {
				return fmt.Errorf("toolchain %s/%s: the checksum must be publisher data, not %q", tc, arch, s.Verification)
			}
			if s.Toolchain != "" {
				return fmt.Errorf("toolchain %s/%s: nested toolchain field", tc, arch)
			}
			if err := ValidateURL(s.URL, hosts); err != nil {
				return fmt.Errorf("toolchain %s/%s: %w", tc, arch, err)
			}
			if version != "" && version != s.Version {
				return fmt.Errorf("toolchain %q has different versions per architecture", tc)
			}
			version = s.Version
		}
	}
	return nil
}
