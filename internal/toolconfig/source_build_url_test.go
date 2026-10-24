package toolconfig

import (
	"strings"
	"testing"
)

func setSourceURL(c *Config, arch, url string) {
	s := c.Lock.Platforms[arch]["sqlite3"]
	s.URL = url
	c.Lock.Platforms[arch]["sqlite3"] = s
}

func setSourceURLBoth(c *Config, url string) {
	setSourceURL(c, "amd64", url)
	setSourceURL(c, "arm64", url)
}

func setToolchainURL(c *Config, arch, url string) {
	s := c.Lock.Toolchains["zig-cc"][arch]
	s.URL = url
	c.Lock.Toolchains["zig-cc"][arch] = s
}

// A lock URL must be derivable from the catalog (base_url, asset template,
// {numeric}) and the version, not merely sit on an allowed host.
func TestSourceBuildLockURLIsBoundToTheRecipe(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		want   string
	}{
		"other path under the base": {func(c *Config) { setSourceURLBoth(c, "https://www.sqlite.org/evil/sqlite-amalgamation-3540100.zip") }, "URL"},
		"numeric of another version": {func(c *Config) {
			setSourceURLBoth(c, "https://www.sqlite.org/2026/sqlite-amalgamation-3540200.zip")
		}, "URL"},
		"other asset name":     {func(c *Config) { setSourceURLBoth(c, "https://www.sqlite.org/2026/sqlite-tools-3540100.zip") }, "URL"},
		"year not four digits": {func(c *Config) { setSourceURLBoth(c, "https://www.sqlite.org/26/sqlite-amalgamation-3540100.zip") }, "URL"},
		"nested path":          {func(c *Config) { setSourceURLBoth(c, "https://www.sqlite.org/2026/a/sqlite-amalgamation-3540100.zip") }, "URL"},
		"dot-dot year":         {func(c *Config) { setSourceURLBoth(c, "https://www.sqlite.org/../sqlite-amalgamation-3540100.zip") }, "URL"},
		"no year":              {func(c *Config) { setSourceURLBoth(c, "https://www.sqlite.org/sqlite-amalgamation-3540100.zip") }, "URL"},
		"base prefix only as host prefix": {func(c *Config) {
			setSourceURLBoth(c, "https://www.sqlite.org.evil.example/2026/sqlite-amalgamation-3540100.zip")
		}, "host"},
		"second allowed host": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.AllowedHosts = []string{"www.sqlite.org", "mirror.example"}
			c.Catalog.Packages["sqlite3"] = p
			setSourceURLBoth(c, "https://mirror.example/2026/sqlite-amalgamation-3540100.zip")
		}, "URL"},
		"architectures differ in source": {func(c *Config) {
			setSourceURL(c, "arm64", "https://www.sqlite.org/2025/sqlite-amalgamation-3540100.zip")
		}, "differ"},
		"architectures differ in hash": {func(c *Config) {
			s := c.Lock.Platforms["arm64"]["sqlite3"]
			s.SHA256 = h("9")
			c.Lock.Platforms["arm64"]["sqlite3"] = s
		}, "differ"},
		"toolchain other version in path": {func(c *Config) {
			setToolchainURL(c, "amd64", "https://ziglang.org/download/0.16.0/zig-x86_64-linux-0.17.0.tar.xz")
		}, "toolchain"},
		"toolchain wrong arch": {func(c *Config) {
			setToolchainURL(c, "arm64", "https://ziglang.org/download/0.17.0/zig-x86_64-linux-0.17.0.tar.xz")
		}, "toolchain"},
		"toolchain other file": {func(c *Config) {
			setToolchainURL(c, "amd64", "https://ziglang.org/download/0.17.0/zig-x86_64-linux-0.17.0.zip")
		}, "toolchain"},
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(t, sqliteConfig(t))
			tc.mutate(&c)
			c.Bind()
			err := c.Validate()
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestSourceBuildLockURLAcceptsDerivedForms(t *testing.T) {
	c := clone(t, sqliteConfig(t))
	setSourceURLBoth(&c, "https://www.sqlite.org/2025/sqlite-amalgamation-3540100.zip")
	c.Bind()
	if err := c.Validate(); err != nil {
		t.Fatalf("a different year is data from the product listing: %v", err)
	}
}

func TestNumericVersion(t *testing.T) {
	for in, want := range map[string]string{"3.54.1": "3540100", "3.9.0": "3090000", "3.54.12": "3541200", "3.0.0": "3000000"} {
		got, err := NumericVersion(in)
		if err != nil || got != want {
			t.Fatalf("%s: %q %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "3.54", "3.100.0", "3.5.100", "10.0.0", "x.1.2", "3.54.1-rc1"} {
		if _, err := NumericVersion(in); err == nil {
			t.Fatalf("accepted %q", in)
		}
	}
}

func TestMultiSourceNestedDotDotAndSecondHost(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		want   string
	}{
		"nested dot-dot file": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.Files["a/../x.c"] = "x.c" })
			editLocked(c, func(l *LockedSource) { l.Files["a/../x.c"] = l.Files["linenoise.c"] })
		}, "unsafe"},
		"template on the package host, not the extra source host": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.URL = "https://www.sqlite.org/antirez/linenoise/{commit}/{file}" })
			relockHost(c, "https://www.sqlite.org/antirez/linenoise/")
		}, "template"},
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(t, linenoiseConfig(t))
			tc.mutate(&c)
			c.Bind()
			err := c.Validate()
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func relockHost(c *Config, prefix string) {
	editLocked(c, func(l *LockedSource) {
		for k, f := range l.Files {
			f.URL = prefix + linenoiseCommit + "/" + k
			l.Files[k] = f
		}
	})
}
