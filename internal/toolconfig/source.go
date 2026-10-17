package toolconfig

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

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

// SourceFetchHosts are the permitted first-hop hosts of an upstream tarball
// template (data-model.md, "Fetch hosts"). The list grows only by a code change
// with a test; redirect targets are checked per hop by the fetcher, not here.
var SourceFetchHosts = []string{
	"api.github.com",
	"codeberg.org",
	"github.com",
	"hackage.haskell.org",
	"proxy.golang.org",
	"static.crates.io",
}

// Bounds on the free-form parts of a source block.
const (
	maxSourceEntries = 64  // entries in licenses, excluded and static_system_libs
	maxSourceText    = 200 // bytes in one license, list entry or version_regex
)

var (
	sourceRepository = regexp.MustCompile(`^https://[a-z0-9][a-z0-9.-]*/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	sourceLicenseID  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]*$`)
	encodedControl   = regexp.MustCompile(`(?i)%(?:[01][0-9a-f]|7f)`)
)

// validSourceText refuses empty, over-long and non-printable text: control
// characters and format characters (bidi overrides, zero width characters).
func validSourceText(s string) bool {
	return s != "" && len(s) <= maxSourceText && !strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
	})
}

// validSourceTexts requires a bounded list of distinct valid texts.
func validSourceTexts(values []string) bool {
	if len(values) > maxSourceEntries {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !validSourceText(v) || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}

// validSourceLicense accepts an SPDX license expression shape: identifiers
// joined by single AND, OR or WITH operators, with balanced parentheses. It does
// not check identifiers against the SPDX list.
func validSourceLicense(expression string) bool {
	if !validSourceText(expression) {
		return false
	}
	depth, wantID := 0, true
	for _, token := range strings.Split(expression, " ") {
		for strings.HasPrefix(token, "(") {
			if !wantID {
				return false
			}
			depth++
			token = token[1:]
		}
		closing := 0
		for strings.HasSuffix(token, ")") {
			closing++
			token = token[:len(token)-1]
		}
		if wantID {
			if !sourceLicenseID.MatchString(token) || token == "AND" || token == "OR" || token == "WITH" {
				return false
			}
		} else {
			if closing > 0 || token != "AND" && token != "OR" && token != "WITH" {
				return false
			}
		}
		wantID = !wantID
		depth -= closing
		if depth < 0 || closing > 0 && wantID {
			return false
		}
	}
	return depth == 0 && !wantID
}

func validSourceRepository(raw string) bool {
	if !sourceRepository.MatchString(raw) {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(raw, "https://"), "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// validSourceTarball checks a URL template whose only placeholders are
// {version} and {lock_version}.
func validSourceTarball(template string) bool {
	expanded := strings.NewReplacer("{version}", "1.2.3", "{lock_version}", "v1.2.3").Replace(template)
	if !strings.Contains(template, "{version}") && !strings.Contains(template, "{lock_version}") {
		return false
	}
	// Empty queries and fragments vanish when parsed, so refuse the markers.
	if !validSourceText(expanded) || strings.ContainsAny(expanded, "{}\\ ?#") || encodedControl.MatchString(expanded) {
		return false
	}
	u, err := url.Parse(expanded)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || !slices.Contains(SourceFetchHosts, u.Host) {
		return false
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == ".." || segment == "." {
			return false
		}
	}
	return true
}

func validateSource(name string, s *Source) error {
	switch s.Obligation {
	case "none", "source-offer", "file-level-source", "undetermined":
	default:
		return fmt.Errorf("invalid source obligation %q for %s", shorten(s.Obligation), name)
	}
	if s.Basis != "" && s.Basis != "declared" && s.Basis != "scanned" {
		return fmt.Errorf("invalid source basis %q for %s", shorten(s.Basis), name)
	}
	copyleft := s.Obligation == "source-offer" || s.Obligation == "file-level-source"
	if s.Obligation != "undetermined" {
		if len(s.Licenses) == 0 || s.Basis == "" {
			return fmt.Errorf("source block of %s requires licenses and basis", name)
		}
	}
	if len(s.Licenses) > maxSourceEntries {
		return fmt.Errorf("source block of %s lists too many licenses", name)
	}
	for i, l := range s.Licenses {
		if !validSourceLicense(l) || i > 0 && l <= s.Licenses[i-1] {
			return fmt.Errorf("source licenses of %s must be valid SPDX-style identifiers, sorted and unique", name)
		}
	}
	if copyleft && (s.Upstream == nil || s.Recipe == nil) {
		return fmt.Errorf("%s source block of %s requires upstream and recipe", shorten(s.Obligation), name)
	}
	if s.Recipe != nil && s.Upstream == nil {
		return fmt.Errorf("source recipe of %s requires upstream", name)
	}
	if s.Obligation == "none" && s.Recipe != nil {
		return fmt.Errorf("source obligation none forbids a recipe for %s", name)
	}
	if s.Upstream != nil {
		if !validSourceRepository(s.Upstream.Repository) {
			return fmt.Errorf("invalid source repository for %s", name)
		}
		if !validSourceTarball(s.Upstream.Tarball) {
			return fmt.Errorf("invalid source tarball template for %s", name)
		}
	}
	if s.Recipe != nil {
		if err := validateSourceRecipe(name, s.Recipe); err != nil {
			return err
		}
	}
	if s.VersionRegex != "" {
		re, err := regexp.Compile(s.VersionRegex)
		if len(s.VersionRegex) > maxSourceText || err != nil || re.NumSubexp() != 1 {
			return fmt.Errorf("source version_regex of %s must compile and have exactly one capture group", name)
		}
	}
	return nil
}

func validateSourceRecipe(name string, r *SourceRecipe) error {
	switch r.Kind {
	case "upstream-tarball-only", "cargo-vendor", "go-mod-download", "cabal-freeze":
	case "custom":
		return fmt.Errorf("source recipe custom is reserved and refused for %s", name)
	default:
		return fmt.Errorf("invalid source recipe kind %q for %s", shorten(r.Kind), name)
	}
	if !validSourceTexts(r.Excluded) || !validSourceTexts(r.StaticSystemLibs) {
		return fmt.Errorf("invalid source recipe text for %s", name)
	}
	if r.Kind != "cabal-freeze" {
		if r.FreezeSource != "" || r.FreezeFile != "" {
			return fmt.Errorf("freeze fields are only valid for cabal-freeze in %s", name)
		}
		return nil
	}
	if (r.FreezeSource == "") == (r.FreezeFile == "") {
		return fmt.Errorf("cabal-freeze requires exactly one of freeze_source and freeze_file for %s", name)
	}
	if r.FreezeSource != "" && !validPath(r.FreezeSource) || r.FreezeFile != "" && (!validPath(r.FreezeFile) || strings.Contains(r.FreezeFile, "/")) {
		return fmt.Errorf("unsafe freeze path for %s", name)
	}
	return nil
}

var sourceProductName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ValidateProductNames enforces the Spec 026 product rules for the catalog
// package names, which are the mirror components: every name is a valid product
// id and no release tag prefix "<a>-v" is a prefix of another "<b>-v", so that
// components such as gh and gh-v2 could not coexist.
func ValidateProductNames(names []string) error {
	for _, name := range names {
		if !sourceProductName.MatchString(name) {
			return fmt.Errorf("package name %q is not a valid product name", name)
		}
	}
	for _, a := range names {
		for _, b := range names {
			if a != b && strings.HasPrefix(b+"-v", a+"-v") {
				return fmt.Errorf("product tag prefixes overlap: %s-v is a prefix of %s-v", a, b)
			}
		}
	}
	return nil
}

var (
	defaultSourceVersion = regexp.MustCompile(`^v?([0-9]{1,9}(?:\.[0-9]{1,9}){0,3})$`)
	numericSourceVersion = regexp.MustCompile(`^[0-9]{1,9}(?:\.[0-9]{1,9}){0,3}$`)
)

// SourceVersion derives the numeric mirror version V from a lock version LV
// (naming table): the package's version_regex, or the default rule when it has
// none, must match the whole lock version (full-match semantics, so a
// pattern cannot silently drop a prerelease suffix) and yield one to four dotted groups of at most nine digits. Leading
// zeros inside a group are allowed. Any other outcome is "version-shape".
func (p Package) SourceVersion(lockVersion string) (string, error) {
	re := defaultSourceVersion
	if p.Source != nil && p.Source.VersionRegex != "" {
		var err error
		if re, err = regexp.Compile(p.Source.VersionRegex); err != nil || re.NumSubexp() != 1 {
			return "", fmt.Errorf("version-shape: invalid version_regex")
		}
	}
	// The pattern must match the whole lock version, not a substring of it.
	m := re.FindStringSubmatch(lockVersion)
	if m == nil || m[0] != lockVersion || !numericSourceVersion.MatchString(m[1]) {
		return "", fmt.Errorf("version-shape: lock version %q does not yield a numeric version", shorten(lockVersion))
	}
	return m[1], nil
}

// shorten bounds a value echoed in an error message.
func shorten(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}
