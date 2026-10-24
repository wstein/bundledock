package toolconfig

import (
	"strings"
	"testing"
)

const linenoiseCommit = "a473823d74b93eab2ba83480df16ed37617493f2"

// linenoiseConfig is the complete sqlite3 example of the design, including the
// commit-pinned linenoise extra source. The hashes are placeholders.
func linenoiseConfig(t *testing.T) Config {
	t.Helper()
	c := clone(t, sqliteConfig(t))
	p := c.Catalog.Packages["sqlite3"]
	p.AllowedHosts = []string{"www.sqlite.org", "raw.githubusercontent.com"}
	p.Build.Sources = []string{"shell.c", "sqlite3.c", "linenoise.c"}
	p.Build.Defines = append([]string{"HAVE_LINENOISE=1"}, p.Build.Defines...)
	p.Build.ExtraSources = []string{"linenoise"}
	p.Notices = append(p.Notices, "licenses/sqlite3/linenoise/LICENSE")
	p.ExtraSources = map[string]ExtraSource{"linenoise": {
		Repository:   "antirez/linenoise",
		Commit:       linenoiseCommit,
		Files:        map[string]string{"linenoise.c": "linenoise.c", "linenoise.h": "linenoise.h", "LICENSE": "LICENSE"},
		URL:          "https://raw.githubusercontent.com/antirez/linenoise/{commit}/{file}",
		AllowedHosts: []string{"raw.githubusercontent.com"},
	}}
	c.Catalog.Packages["sqlite3"] = p
	files := map[string]LockedFile{}
	for _, f := range []string{"linenoise.c", "linenoise.h", "LICENSE"} {
		files[f] = LockedFile{URL: "https://raw.githubusercontent.com/antirez/linenoise/" + linenoiseCommit + "/" + f, SHA256: h("5"), Verification: "tls-download-sha256"}
	}
	c.Lock.ExtraSources = map[string]LockedSource{"sqlite3/linenoise": {Commit: linenoiseCommit, Files: files}}
	c.Bind()
	return c
}

func editExtra(c *Config, f func(e *ExtraSource)) {
	p := c.Catalog.Packages["sqlite3"]
	e := p.ExtraSources["linenoise"]
	f(&e)
	p.ExtraSources["linenoise"] = e
}

func editLocked(c *Config, f func(l *LockedSource)) {
	l := c.Lock.ExtraSources["sqlite3/linenoise"]
	f(&l)
	c.Lock.ExtraSources["sqlite3/linenoise"] = l
}

func TestMultiSourceDesignExampleValidates(t *testing.T) {
	c := linenoiseConfig(t)
	if err := c.Validate(); err != nil {
		t.Fatalf("the design's complete sqlite3 example is rejected: %v", err)
	}
}

func TestMultiSourceRejectedShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		want   string
	}{
		"commit is a branch": {func(c *Config) { editExtra(c, func(e *ExtraSource) { e.Commit = "main" }) }, "40-hex"},
		"commit is short":    {func(c *Config) { editExtra(c, func(e *ExtraSource) { e.Commit = "a473823" }) }, "40-hex"},
		"commit upper case":  {func(c *Config) { editExtra(c, func(e *ExtraSource) { e.Commit = strings.ToUpper(linenoiseCommit) }) }, "40-hex"},
		"lock commit differs": {func(c *Config) {
			editLocked(c, func(l *LockedSource) { l.Commit = strings.Repeat("b", 40) })
		}, "commit"},
		"lock commit a branch": {func(c *Config) { editLocked(c, func(l *LockedSource) { l.Commit = "main" }) }, "commit"},
		"url commit differs": {func(c *Config) {
			editLocked(c, func(l *LockedSource) {
				f := l.Files["linenoise.c"]
				f.URL = strings.Replace(f.URL, linenoiseCommit, strings.Repeat("b", 40), 1)
				l.Files["linenoise.c"] = f
			})
		}, "URL"},
		"url is a branch": {func(c *Config) {
			editLocked(c, func(l *LockedSource) {
				f := l.Files["linenoise.c"]
				f.URL = strings.Replace(f.URL, linenoiseCommit, "master", 1)
				l.Files["linenoise.c"] = f
			})
		}, "URL"},
		"url template without commit": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.URL = "https://raw.githubusercontent.com/antirez/linenoise/master/{file}" })
		}, "placeholders"},
		"url template without file": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) {
				e.URL = "https://raw.githubusercontent.com/antirez/linenoise/{commit}/linenoise.c"
			})
		}, "placeholders"},
		"url template unknown placeholder": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.URL += "{arch}" })
		}, "placeholders"},
		"extra host not allowed": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.AllowedHosts = []string{"example.com"} })
		}, "host"},
		"extra host missing from package": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.AllowedHosts = []string{"www.sqlite.org"}
			c.Catalog.Packages["sqlite3"] = p
		}, "host"},
		"no host list":   {func(c *Config) { editExtra(c, func(e *ExtraSource) { e.AllowedHosts = nil }) }, "host"},
		"bad repository": {func(c *Config) { editExtra(c, func(e *ExtraSource) { e.Repository = "../x" }) }, "repository"},
		"no files":       {func(c *Config) { editExtra(c, func(e *ExtraSource) { e.Files = nil }) }, "files"},
		"unsafe file name": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.Files["../evil.c"] = "evil.c" })
		}, "file"},
		"unsafe build name": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.Files["linenoise.h"] = "../linenoise.h" })
		}, "file"},
		"missing LICENSE": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { delete(e.Files, "LICENSE") })
			editLocked(c, func(l *LockedSource) { delete(l.Files, "LICENSE") })
		}, "LICENSE"},
		"duplicate build file names": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.Files["linenoise.h"] = "linenoise.c" })
		}, "duplicate"},
		"duplicate across sources": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.ExtraSources["other"] = ExtraSource{Repository: "a/b", Commit: strings.Repeat("c", 40), Files: map[string]string{"linenoise.c": "linenoise.c", "LICENSE": "LICENSE2"},
				URL: "https://raw.githubusercontent.com/a/b/{commit}/{file}", AllowedHosts: []string{"raw.githubusercontent.com"}}
			p.Build.ExtraSources = []string{"linenoise", "other"}
			c.Catalog.Packages["sqlite3"] = p
			c.Lock.ExtraSources["sqlite3/other"] = LockedSource{Commit: strings.Repeat("c", 40), Files: map[string]LockedFile{
				"linenoise.c": {URL: "https://raw.githubusercontent.com/a/b/" + strings.Repeat("c", 40) + "/linenoise.c", SHA256: h("5"), Verification: "tls-download-sha256"},
				"LICENSE":     {URL: "https://raw.githubusercontent.com/a/b/" + strings.Repeat("c", 40) + "/LICENSE", SHA256: h("5"), Verification: "tls-download-sha256"}}}
		}, "duplicate"},
		"declared but not listed": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.ExtraSources = nil
			c.Catalog.Packages["sqlite3"] = p
			delete(c.Lock.ExtraSources, "sqlite3/linenoise")
		}, "not listed"},
		"listed but not declared": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.ExtraSources = []string{"linenoise", "ghost"}
			c.Catalog.Packages["sqlite3"] = p
		}, "declares"},
		"listed twice": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.ExtraSources = []string{"linenoise", "linenoise"}
			c.Catalog.Packages["sqlite3"] = p
		}, "linenoise"},
		"locked but not listed": {func(c *Config) {
			c.Lock.ExtraSources["sqlite3/other"] = c.Lock.ExtraSources["sqlite3/linenoise"]
		}, "not listed"},
		"locked for an unknown package": {func(c *Config) {
			c.Lock.ExtraSources["ghost/linenoise"] = c.Lock.ExtraSources["sqlite3/linenoise"]
		}, "not listed"},
		"listed but unlocked": {func(c *Config) { c.Lock.ExtraSources = nil }, "not locked"},
		"locked file missing": {func(c *Config) {
			editLocked(c, func(l *LockedSource) { delete(l.Files, "linenoise.h") })
		}, "files"},
		"locked file extra": {func(c *Config) {
			editLocked(c, func(l *LockedSource) { l.Files["extra.c"] = l.Files["linenoise.c"] })
		}, "files"},
		"file unhashed": {func(c *Config) {
			editLocked(c, func(l *LockedSource) { f := l.Files["linenoise.c"]; f.SHA256 = ""; l.Files["linenoise.c"] = f })
		}, "sha256"},
		"file short hash": {func(c *Config) {
			editLocked(c, func(l *LockedSource) { f := l.Files["linenoise.c"]; f.SHA256 = "abc"; l.Files["linenoise.c"] = f })
		}, "sha256"},
		"file publisher class": {func(c *Config) {
			editLocked(c, func(l *LockedSource) {
				f := l.Files["linenoise.c"]
				f.Verification = "publisher-sha256"
				l.Files["linenoise.c"] = f
			})
		}, "tls-download-sha256"},
		"file no class": {func(c *Config) {
			editLocked(c, func(l *LockedSource) { f := l.Files["linenoise.c"]; f.Verification = ""; l.Files["linenoise.c"] = f })
		}, "tls-download-sha256"},
		"file url host": {func(c *Config) {
			editLocked(c, func(l *LockedSource) {
				f := l.Files["linenoise.c"]
				f.URL = "https://example.com/linenoise.c"
				l.Files["linenoise.c"] = f
			})
		}, "URL"},
		"url not https": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.URL = "http://raw.githubusercontent.com/antirez/linenoise/{commit}/{file}" })
			editLocked(c, func(l *LockedSource) {
				for k, f := range l.Files {
					f.URL = "http://raw.githubusercontent.com/antirez/linenoise/" + linenoiseCommit + "/" + k
					l.Files[k] = f
				}
			})
		}, "invalid official URL"},
		"build name is an option": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.Files["linenoise.h"] = "-include" })
		}, "unsafe"},
		"build name with a directory": {func(c *Config) {
			editExtra(c, func(e *ExtraSource) { e.Files["linenoise.h"] = "sub/linenoise.h" })
		}, "unsafe"},
		"locked file renamed": {func(c *Config) {
			editLocked(c, func(l *LockedSource) {
				f := l.Files["LICENSE"]
				f.URL = strings.TrimSuffix(f.URL, "LICENSE") + "COPYING"
				delete(l.Files, "LICENSE")
				l.Files["COPYING"] = f
			})
		}, "differ"},
		"bad extra source name": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.ExtraSources["Bad Name"] = p.ExtraSources["linenoise"]
			delete(p.ExtraSources, "linenoise")
			p.Build.ExtraSources = []string{"Bad Name"}
			c.Lock.ExtraSources["sqlite3/Bad Name"] = c.Lock.ExtraSources["sqlite3/linenoise"]
			delete(c.Lock.ExtraSources, "sqlite3/linenoise")
		}, "invalid"},
		"extra sources on prebuilt": {func(c *Config) {
			p := prebuiltPackage()
			p.ExtraSources = c.Catalog.Packages["sqlite3"].ExtraSources
			c.Catalog.Packages["sqlite3"] = p
			c.Lock.Toolchains, c.Lock.ExtraSources = nil, nil
			s := Spec{Version: "3.54.1", URL: "https://github.com/a/b/x", SHA256: h("2"), Verification: "publisher-sha256"}
			c.Lock.Platforms["amd64"]["sqlite3"], c.Lock.Platforms["arm64"]["sqlite3"] = s, s
		}, "extra_sources blocks are only valid for the source-build"},
		"HAVE_LINENOISE without the source": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"shell.c", "sqlite3.c"}
			p.Build.ExtraSources = nil
			p.ExtraSources = nil
			c.Catalog.Packages["sqlite3"] = p
			c.Lock.ExtraSources = nil
		}, "HAVE_LINENOISE"},
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

func TestMultiSourceMainArchiveNeverTLSClass(t *testing.T) {
	c := clone(t, linenoiseConfig(t))
	for _, arch := range []string{"amd64", "arm64"} {
		s := c.Lock.Platforms[arch]["sqlite3"]
		s.Verification, s.SHA3256 = "tls-download-sha256", ""
		c.Lock.Platforms[arch]["sqlite3"] = s
	}
	c.Bind()
	if err := c.Validate(); err == nil {
		t.Fatal("tls-download-sha256 accepted for the main archive")
	}
}

func TestMultiSourceInputsProbeAndLoad(t *testing.T) {
	c := linenoiseConfig(t)
	dir := t.TempDir()
	writeBundle(t, dir, "sqlite3>=3.54.1,<4.0.0\n", c)
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Lock.ExtraSources["sqlite3/linenoise"].Commit != linenoiseCommit {
		t.Fatal("extra sources not loaded")
	}
	if _, err := LoadInputs(dir); err != nil {
		t.Fatal(err)
	}
}
