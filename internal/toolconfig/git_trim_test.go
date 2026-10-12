package toolconfig

import (
	"encoding/json"
	"testing"
)

func TestGitTrimSourceException(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["git-trim"]
	if !ok {
		t.Fatal("git-trim source exception is missing")
	}
	if p.Repository != "foriequal0/git-trim" || p.Install.Profile != "git-trim-source" || p.Install.Members["git-trim"] != "bin/git-trim" {
		t.Fatal("invalid bounded source recipe")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		s := c.Lock.Platforms[arch]["git-trim"]
		if s.Version != "v0.4.3" || s.URL != "https://codeload.github.com/foriequal0/git-trim/tar.gz/65288617a637029573a5c3517abcb95b050ea02b" || s.SHA256 != "dffccf97f3801976a647f5d03629d49f17d3b930b322ee1056239084093b4e53" {
			t.Fatalf("invalid %s source pin: %+v", arch, s)
		}
	}
}

func TestGitTrimExceptionRejectsBroaderRecipes(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) {
			p := c.Catalog.Packages["git-trim"]
			p.Repository = "other/git-trim"
			c.Catalog.Packages["git-trim"] = p
		},
		func(c *Config) {
			p := c.Catalog.Packages["gh"]
			p.Install.Profile = GitTrimProfile
			c.Catalog.Packages["gh"] = p
		},
		func(c *Config) {
			s := c.Lock.Platforms["amd64"]["git-trim"]
			s.Source.Commit = "0000000000000000000000000000000000000000"
			c.Lock.Platforms["amd64"]["git-trim"] = s
		},
		func(c *Config) {
			s := c.Lock.Platforms["amd64"]["git-trim"]
			s.Source.Builder = "rust:latest"
			c.Lock.Platforms["amd64"]["git-trim"] = s
		},
		func(c *Config) {
			s := c.Lock.Platforms["amd64"]["git-trim"]
			s.Source = nil
			c.Lock.Platforms["amd64"]["git-trim"] = s
		},
		func(c *Config) {
			s := c.Lock.Platforms["amd64"]["gh"]
			s.Source = GitTrimSpec().Source
			c.Lock.Platforms["amd64"]["gh"] = s
		},
	} {
		c, err := Load("../../bundles/toolbox")
		if err != nil {
			t.Fatal(err)
		}
		mutate(c)
		c.Bind()
		if c.Validate() == nil {
			b, _ := json.Marshal(c)
			t.Fatalf("broader source recipe accepted: %s", b)
		}
	}
}
