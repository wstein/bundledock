package toolconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// ToolboxDirectory is the repository-relative home of the default tool bundle.
const ToolboxDirectory = "bundles/toolbox"

const ToolboxLock = ToolboxDirectory + "/tools.lock.json"

type Spec struct {
	Version      string `json:"version"`
	URL          string `json:"url"`
	SHA256       string `json:"sha256,omitempty"`
	SHA512       string `json:"sha512,omitempty"`
	Verification string `json:"verification,omitempty"`
}
type Lock struct {
	Schema        int                        `json:"schema"`
	DesiredDigest string                     `json:"desired_digest"`
	CatalogDigest string                     `json:"catalog_digest"`
	Platforms     map[string]map[string]Spec `json:"platforms"`
}
type Catalog struct {
	Schema   int                `json:"schema"`
	Packages map[string]Package `json:"packages"`
}
type Package struct {
	Provider      string            `json:"provider"`
	Repository    string            `json:"repository,omitempty"`
	TagPrefix     string            `json:"tag_prefix,omitempty"`
	BaseURL       string            `json:"base_url,omitempty"`
	MetadataURL   string            `json:"metadata_url,omitempty"`
	Normalization string            `json:"normalization"`
	Architectures map[string]string `json:"architectures"`
	Asset         string            `json:"asset"`
	AllowedHosts  []string          `json:"allowed_hosts"`
	Verification  string            `json:"verification"`
	Sidecar       string            `json:"sidecar,omitempty"`
	Install       Install           `json:"install"`
	Notices       []string          `json:"notices,omitempty"`
}
type Install struct {
	Profile            string            `json:"profile,omitempty"`
	Kind               string            `json:"kind"`
	Members            map[string]string `json:"members,omitempty"`
	Aliases            map[string]string `json:"aliases,omitempty"`
	Preserve           bool              `json:"preserve,omitempty"`
	ArchiveNotices     []string          `json:"archive_notices,omitempty"`
	ExcludeDirectories []string          `json:"exclude_directories,omitempty"`
}

// Excludes reports whether a normalized archive path is in an omitted directory.
// Directory names are exact relative paths, without glob or prefix matching.
func (i Install) Excludes(name string) bool {
	for _, directory := range i.ExcludeDirectories {
		if name == directory || strings.HasPrefix(name, directory+"/") {
			return true
		}
	}
	return false
}

type Config struct {
	Desired Desired
	Catalog Catalog
	Lock    Lock
}

func Decode(r io.Reader, value any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}
func digest(value any) string {
	data, _ := json.Marshal(value)
	var canonical any
	_ = json.Unmarshal(data, &canonical)
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(canonical)
	data = bytes.TrimSuffix(b.Bytes(), []byte("\n"))
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func (c Config) DesiredDigest() string {
	m := map[string]string{}
	for n, v := range c.Desired {
		m[n] = v.String()
	}
	return digest(m)
}
func (c Config) CatalogDigest() string { return digest(c.Catalog) }
func (c *Config) Bind() {
	c.Lock.Schema = 2
	c.Lock.DesiredDigest = c.DesiredDigest()
	c.Lock.CatalogDigest = c.CatalogDigest()
}
func Load(directory string) (*Config, error) { return load(directory, true) }

// LoadForUpgrade permits changed desired inputs; the newly resolved lock must pass Validate.
func LoadForUpgrade(directory string) (*Config, error) { return load(directory, false) }

// LoadInputs validates trusted declarative inputs independently of an existing lock.
func LoadInputs(directory string) (*Config, error) {
	c := new(Config)
	f, err := os.Open(filepath.Join(directory, "tools.txt"))
	if err != nil {
		return nil, err
	}
	c.Desired, err = ParseDesired(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	f, err = os.Open(filepath.Join(directory, "tools.packages.json"))
	if err != nil {
		return nil, err
	}
	err = Decode(f, &c.Catalog)
	f.Close()
	if err != nil {
		return nil, err
	}
	// Validate recipe grammar with synthetic pins; no URLs are fetched or executed.
	probe := *c
	probe.Lock = Lock{Platforms: map[string]map[string]Spec{"amd64": {}, "arm64": {}}}
	for n, p := range c.Catalog.Packages {
		host := "invalid"
		if len(p.AllowedHosts) > 0 {
			host = p.AllowedHosts[0]
		}
		probeVersion := p.TagPrefix + c.Desired[n].Lower.String()
		if p.Normalization == "tmux-release" {
			probeVersion = "v3.8"
		}
		s := Spec{Version: probeVersion, URL: "https://" + host + "/", SHA256: strings.Repeat("0", 64), Verification: "tls-download-sha256"}
		for _, a := range []string{"amd64", "arm64"} {
			probe.Lock.Platforms[a][n] = s
		}
	}
	probe.Bind()
	// Synthetic pins validate recipe grammar; desired membership is checked only
	// for real resolved pins. A tmux suffix cannot be inferred from every semver.
	if err := probe.validate(false); err != nil {
		return nil, err
	}
	return c, nil
}
func load(directory string, strict bool) (*Config, error) {
	c, err := LoadInputs(directory)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(directory, "tools.lock.json"))
	if err != nil {
		return nil, err
	}
	err = Decode(f, &c.Lock)
	f.Close()
	if err != nil {
		return nil, err
	}
	if err := c.validate(strict); err != nil {
		return nil, err
	}
	return c, nil
}

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func validPath(p string) bool {
	return p != "" && p != "." && !strings.HasPrefix(p, "/") && !strings.Contains(p, "\\") && !strings.ContainsAny(p, "\r\n\x00") && path.Clean(p) == p && !strings.HasPrefix(p, "../") && p != ".."
}
func ValidateURL(raw string, hosts []string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return fmt.Errorf("invalid official URL %q", raw)
	}
	for _, host := range hosts {
		if u.Host == host {
			return nil
		}
	}
	return fmt.Errorf("URL host is not allowed: %s", u.Host)
}
func ValidateSpec(s Spec) error {
	if s.Version == "" {
		return fmt.Errorf("missing version")
	}
	sum, size := s.SHA256, 32
	if s.SHA512 != "" {
		sum, size = s.SHA512, 64
	}
	if (s.SHA256 == "") == (s.SHA512 == "") {
		return fmt.Errorf("exactly one checksum required")
	}
	b, err := hex.DecodeString(sum)
	if err != nil || len(b) != size {
		return fmt.Errorf("invalid checksum")
	}
	switch s.Verification {
	case "publisher-sha256", "publisher-sha512", "tls-download-sha256":
	default:
		return fmt.Errorf("invalid verification provenance %q", s.Verification)
	}
	if s.Verification == "publisher-sha512" && s.SHA512 == "" {
		return fmt.Errorf("SHA512 verification requires SHA512")
	}
	return nil
}
func (p Package) Version(tag string) (string, error) {
	v := strings.TrimPrefix(tag, p.TagPrefix)
	if p.Normalization == "tmux-release" {
		match := regexp.MustCompile(`^([0-9]+)\.([0-9]+)([a-z]?)$`).FindStringSubmatch(v)
		if match == nil {
			return "", fmt.Errorf("unsupported tmux release %q", tag)
		}
		patch := 0
		if match[3] != "" {
			patch = int(match[3][0]-'a') + 1
		}
		v = fmt.Sprintf("%s.%s.%d", match[1], match[2], patch)
	}
	_, err := ParseVersion(v)
	return v, err
}
func (c Config) Validate() error { return c.validate(true) }
func (c Config) validate(strict bool) error {
	if c.Catalog.Schema != 1 || c.Lock.Schema != 2 {
		return fmt.Errorf("unsupported catalog/lock schema")
	}
	if strict && (c.Lock.DesiredDigest != c.DesiredDigest() || c.Lock.CatalogDigest != c.CatalogDigest()) {
		return fmt.Errorf("tools.lock.json identity differs from tools.txt or tools.packages.json; run bundledock tools upgrade (desired %s/%s catalog %s/%s)", c.Lock.DesiredDigest, c.DesiredDigest(), c.Lock.CatalogDigest, c.CatalogDigest())
	}
	if len(c.Desired) != len(c.Catalog.Packages) || len(c.Lock.Platforms) != 2 {
		return fmt.Errorf("desired tools, catalog and both lock architectures must agree")
	}
	for name, p := range c.Catalog.Packages {
		constraint, ok := c.Desired[name]
		if !ok {
			return fmt.Errorf("catalog tool %s is absent from tools.txt", name)
		}
		if p.Normalization != "semver" && p.Normalization != "tmux-release" {
			return fmt.Errorf("unknown normalization for %s", name)
		}
		switch p.Provider {
		case "github", "claude", "antigravity":
			if !repositoryName.MatchString(p.Repository) {
				return fmt.Errorf("invalid repository for %s", name)
			}
		default:
			return fmt.Errorf("unknown provider for %s", name)
		}

		if p.Normalization == "tmux-release" && (name != "tmux" || p.Provider != "github" || p.Repository != "tmux/tmux-builds" || p.TagPrefix != "v") {
			return fmt.Errorf("tmux-release is only supported for official tmux builds")
		}
		if p.Provider == "claude" && (p.BaseURL == "" || p.Install.Kind != "raw") {
			return fmt.Errorf("Claude requires official base URL and raw binary recipe")
		}
		if p.Provider == "antigravity" && p.MetadataURL == "" {
			return fmt.Errorf("Antigravity requires official updater metadata URL")
		}
		for _, template := range []string{p.Asset, p.Sidecar} {
			expanded := strings.NewReplacer("{version}", "1.2.3", "{arch}", "arch", "{asset}", "asset").Replace(template)
			if template != "" && (!validPath(expanded) || strings.ContainsAny(expanded, "{}")) {
				return fmt.Errorf("invalid asset template for %s", name)
			}
		}
		if p.Install.Kind == "raw" && len(p.Install.Members) != 1 {
			return fmt.Errorf("raw recipe requires one executable for %s", name)
		}
		if len(p.Architectures) != 2 || p.Architectures["amd64"] == "" || p.Architectures["arm64"] == "" || p.Asset == "" || len(p.AllowedHosts) == 0 {
			return fmt.Errorf("missing assets/platforms for %s", name)
		}
		if p.Verification != "official-or-download" {
			return fmt.Errorf("unknown verification policy for %s", name)
		}
		for _, u := range []string{p.BaseURL, p.MetadataURL} {
			if u != "" {
				if err := ValidateURL(u, p.AllowedHosts); err != nil {
					return err
				}
			}
		}
		switch p.Install.Kind {
		case "raw", "tar.gz", "tar.xz", "zip":
		default:
			return fmt.Errorf("unknown install kind for %s", name)
		}
		if p.Install.Profile != "" && (p.Install.Profile != "code-server-standalone" || len(c.Catalog.Packages) != 1 || name != "code-server" || p.Provider != "github" || p.Repository != "coder/code-server" || p.Install.Kind != "tar.gz" || !p.Install.Preserve || len(p.Install.Members) != 1 || p.Install.Members["bin/code-server"] != "payload/bin/code-server" || len(p.Install.Aliases) != 0 || len(p.Install.ExcludeDirectories) != 0) {
			return fmt.Errorf("invalid standalone profile for %s", name)
		}
		if len(p.Install.Members) == 0 {
			return fmt.Errorf("missing install members for %s", name)
		}
		for member, target := range p.Install.Members {
			if !validPath(member) || !validPath(target) {
				return fmt.Errorf("unsafe member/target for %s", name)
			}
		}
		for alias, target := range p.Install.Aliases {
			if !validPath(alias) || !validPath(target) {
				return fmt.Errorf("unsafe alias for %s", name)
			}
			found := false
			for _, v := range p.Install.Members {
				if v == target {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("alias target is not installed for %s", name)
			}
		}
		if p.Install.Preserve && p.Install.Kind != "tar.gz" && p.Install.Kind != "tar.xz" {
			return fmt.Errorf("preserve requires tar.gz or tar.xz")
		}
		if len(p.Install.ExcludeDirectories) > 0 && !p.Install.Preserve {
			return fmt.Errorf("directory exclusions require preserve for %s", name)
		}
		for _, directory := range p.Install.ExcludeDirectories {
			if !validPath(directory) {
				return fmt.Errorf("unsafe excluded directory for %s", name)
			}
		}
		for member := range p.Install.Members {
			if p.Install.Excludes(member) {
				return fmt.Errorf("excluded executable member for %s", name)
			}
		}
		for _, notice := range append(append([]string{}, p.Notices...), p.Install.ArchiveNotices...) {
			if !validPath(notice) {
				return fmt.Errorf("unsafe notice for %s", name)
			}
		}
		var first string
		for _, arch := range []string{"amd64", "arm64"} {
			tools, ok := c.Lock.Platforms[arch]
			if !ok || len(tools) != len(c.Desired) {
				return fmt.Errorf("incomplete lock architecture %s", arch)
			}
			s, ok := tools[name]
			if !ok {
				return fmt.Errorf("missing %s/%s", arch, name)
			}
			if err := ValidateSpec(s); err != nil {
				return fmt.Errorf("%s/%s: %w", arch, name, err)
			}
			if err := ValidateURL(s.URL, p.AllowedHosts); err != nil {
				return err
			}
			v, err := p.Version(s.Version)
			if err != nil || strict && !constraint.Contains(v) {
				return fmt.Errorf("%s/%s version %q outside desired range", arch, name, s.Version)
			}
			if first != "" && first != v {
				return fmt.Errorf("different versions for %s architectures", name)
			}
			first = v
		}
	}
	return nil
}
