package toolconfig

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var (
	extraSourceName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	// repoFilePath is a safe repository-relative path: no dot-dot, percent, query or fragment.
	repoFilePath = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*(/[A-Za-z0-9_][A-Za-z0-9_.+-]*)*$`)
	commitSHA    = regexp.MustCompile(`^[0-9a-f]{40}$`)
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

// ExtraSource is a further pinned source of a source-build package: files of
// one commit of a source repository. The publisher offers no checksum for it, so
// the lock pins each file by the builder-computed SHA256 (class
// tls-download-sha256), the named exception of AGENTS.md.
type ExtraSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	// Files maps a repository path to the file name used in the build directory.
	Files map[string]string `json:"files"`
	// URL is a template with {commit} and {file} (the repository path).
	URL          string   `json:"url"`
	AllowedHosts []string `json:"allowed_hosts"`
}

// LockedSource is the lock entry of an ExtraSource, keyed "<tool>/<name>".
type LockedSource struct {
	Commit string                `json:"commit"`
	Files  map[string]LockedFile `json:"files"`
}

// LockedFile pins one extra-source file by URL and builder-computed SHA256.
type LockedFile struct {
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	Verification string `json:"verification"`
}

// Build is the recipe of a provider "source-build" package.
type Build struct {
	Kind          string   `json:"kind"`
	Toolchain     string   `json:"toolchain"`
	ArchivePrefix string   `json:"archive_prefix"`
	Sources       []string `json:"sources"`
	// ExtraSources lists, by name, the package-level extra_sources the recipe consumes.
	ExtraSources []string `json:"extra_sources,omitempty"`
	Defines      []string `json:"defines,omitempty"`
	Cflags       []string `json:"cflags,omitempty"`
	Libs         []string `json:"libs,omitempty"`
	Static       bool     `json:"static"`
	OutputForm   string   `json:"output_form"`
	Output       string   `json:"output"`
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
		if p.Build != nil || p.VersionSource != nil || len(p.ExtraSources) != 0 {
			return fmt.Errorf("%s: the build, version_source and extra_sources blocks are only valid for the source-build provider", name)
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
	if err := validateExtraSources(name, p); err != nil {
		return err
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

func hasCommitSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if commitSHA.MatchString(seg) {
			return true
		}
	}
	return false
}

// extraURL expands an extra source's URL template for one repository path.
func extraURL(e ExtraSource, file string) string {
	return strings.NewReplacer("{commit}", e.Commit, "{file}", file).Replace(e.URL)
}

func validateExtraSources(name string, p Package) error {
	listed := map[string]bool{}
	for _, n := range p.Build.ExtraSources {
		if listed[n] || !extraSourceName.MatchString(n) {
			return fmt.Errorf("%s: invalid or duplicate build.extra_sources entry %q", name, n)
		}
		listed[n] = true
		if _, ok := p.ExtraSources[n]; !ok {
			return fmt.Errorf("%s: build.extra_sources lists %q, which no extra_sources entry declares", name, n)
		}
	}
	local := map[string]bool{}
	exact := map[string]bool{}
	for n, e := range p.ExtraSources {
		if !listed[n] {
			return fmt.Errorf("%s: extra source %q is declared but not listed in build.extra_sources", name, n)
		}
		if !repositoryName.MatchString(e.Repository) || strings.HasPrefix(e.Repository, ".") || strings.Contains(e.Repository, "/.") {
			return fmt.Errorf("%s/%s: invalid extra source repository %q", name, n, e.Repository)
		}
		if !commitSHA.MatchString(e.Commit) {
			return fmt.Errorf("%s/%s: the extra source commit must be a full 40-hex SHA, not %q", name, n, e.Commit)
		}
		if len(e.Files) == 0 {
			return fmt.Errorf("%s/%s: extra source lists no files", name, n)
		}
		for _, h := range e.AllowedHosts {
			ok := false
			for _, ph := range p.AllowedHosts {
				ok = ok || ph == h
			}
			if !ok {
				return fmt.Errorf("%s/%s: extra source host %q is not in the package allowed_hosts", name, n, h)
			}
		}
		template := false
		for _, h := range e.AllowedHosts {
			template = template || e.URL == "https://"+h+"/"+e.Repository+"/{commit}/{file}"
		}
		if !template {
			return fmt.Errorf("%s/%s: extra source URL template must be exactly https://<allowed host>/<repository>/{commit}/{file} with the two placeholders", name, n)
		}
		license := false
		for file, target := range e.Files {
			if !repoFilePath.MatchString(file) || hasCommitSegment(file) || !outputName.MatchString(target) {
				return fmt.Errorf("%s/%s: unsafe extra source file %q -> %q", name, n, file, target)
			}
			if local[strings.ToLower(target)] {
				return fmt.Errorf("%s/%s: duplicate build file name %q across sources", name, n, target)
			}
			local[strings.ToLower(target)] = true
			exact[target] = true
			if err := ValidateURL(extraURL(e, file), e.AllowedHosts); err != nil {
				return fmt.Errorf("%s/%s: %w", name, n, err)
			}
			license = license || file == "LICENSE" || file == "COPYING"
		}
		if !license {
			return fmt.Errorf("%s/%s: extra source must include its LICENSE or COPYING file", name, n)
		}
	}
	for _, d := range p.Build.Defines {
		if d == "HAVE_LINENOISE=1" && !(exact["linenoise.c"] && exact["linenoise.h"]) {
			return fmt.Errorf("%s: HAVE_LINENOISE needs linenoise.c and linenoise.h from an extra source", name)
		}
	}
	return nil
}

// validateLockedExtraSources checks lock.extra_sources against the catalog.
func (c Config) validateLockedExtraSources() error {
	want := map[string]bool{}
	for name, p := range c.Catalog.Packages {
		if p.Provider != "source-build" || p.Build == nil {
			continue
		}
		for _, n := range p.Build.ExtraSources {
			key := name + "/" + n
			want[key] = true
			e, l := p.ExtraSources[n], c.Lock.ExtraSources[key]
			if l.Commit == "" && len(l.Files) == 0 {
				return fmt.Errorf("extra source %s is not locked", key)
			}
			if !commitSHA.MatchString(l.Commit) || l.Commit != e.Commit {
				return fmt.Errorf("extra source %s: the locked commit %q differs from the catalog commit", key, l.Commit)
			}
			if len(l.Files) != len(e.Files) {
				return fmt.Errorf("extra source %s: the locked files differ from the catalog files", key)
			}
			for file, lf := range l.Files {
				if _, ok := e.Files[file]; !ok {
					return fmt.Errorf("extra source %s: the locked files differ from the catalog files (%q)", key, file)
				}
				if lf.URL != extraURL(e, file) {
					return fmt.Errorf("extra source %s: URL of %q is not the catalog template at the pinned commit", key, file)
				}
				if sum, err := hex.DecodeString(lf.SHA256); err != nil || len(sum) != 32 {
					return fmt.Errorf("extra source %s: invalid sha256 for %q", key, file)
				}
				if lf.Verification != "tls-download-sha256" {
					return fmt.Errorf("extra source %s: %q needs class tls-download-sha256, not %q", key, file, lf.Verification)
				}
			}
		}
	}
	for key := range c.Lock.ExtraSources {
		if !want[key] {
			return fmt.Errorf("locked extra source %s is not listed by any source-build recipe", key)
		}
	}
	return nil
}
