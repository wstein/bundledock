package nixbox

import (
	"bundledock/internal/toolconfig"
	"os"
	"strings"
	"testing"
)

func productionCatalog(t *testing.T) ([]byte, *Catalog) {
	t.Helper()
	data, err := os.ReadFile("../../bundles/nixbox/packages.catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	return data, c
}

func TestProductionCatalogShape(t *testing.T) {
	_, c := productionCatalog(t)
	for _, name := range []string{"git", "rsync", "zstd", "less", "openssh", "cacert", "glibc", "gcc-lib"} {
		if p, ok := c.Packages[name]; !ok || p.Provider != ProviderNix {
			t.Errorf("%s must be a nix package", name)
		}
	}
	want := []string{"ast-grep", "delta", "difft", "fastfetch", "hx", "leaf", "zizmor", "zk"}
	got := c.OfficialNames()
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("official shims = %v, want %v", got, want)
	}
	if c.Packages["hx"].Env["HELIX_RUNTIME"] != "lib/helix/runtime" || c.Packages["hx"].Binary != "lib/helix/hx" {
		t.Error("hx must run lib/helix/hx with HELIX_RUNTIME")
	}
}

// The official binaries are installed from bundles/nixbox/tools with the toolbox
// recipes, so every shim must name a file that the tools catalog installs.
func TestProductionOfficialBinariesAreInstalledByTheToolsCatalog(t *testing.T) {
	_, c := productionCatalog(t)
	tools, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckOfficial(c, tools); err != nil {
		t.Fatal(err)
	}
}

func TestCheckOfficialAcceptsAShimOnAnAlias(t *testing.T) {
	_, c := productionCatalog(t)
	tools, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	zk := tools.Catalog.Packages["zk"]
	zk.Install.Aliases = map[string]string{"bin/zk-alias": "bin/zk"}
	tools.Catalog.Packages["zk"] = zk
	alias := *c
	alias.Packages = map[string]Package{}
	for name, p := range c.Packages {
		alias.Packages[name] = p
	}
	p := alias.Packages["zk"]
	p.Binary = "bin/zk-alias"
	alias.Packages["zk"] = p
	// The alias is installed, but bin/zk then has no shim.
	if err := CheckOfficial(&alias, tools); err == nil {
		t.Fatal("accepted a recipe member without a shim")
	}
	if err := CheckOfficial(c, tools); err != nil {
		t.Fatalf("an alias must not need a shim: %v", err)
	}
}

func TestCheckOfficialRefusesAShimWithoutARecipe(t *testing.T) {
	_, c := productionCatalog(t)
	tools, err := toolconfig.Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	// A shim naming a file no recipe installs.
	wrong := *c
	wrong.Packages = map[string]Package{}
	for name, p := range c.Packages {
		wrong.Packages[name] = p
	}
	zk := wrong.Packages["zk"]
	zk.Binary = "bin/not-installed"
	wrong.Packages["zk"] = zk
	if err := CheckOfficial(&wrong, tools); err == nil {
		t.Error("accepted a shim whose binary no recipe installs")
	}
	// A recipe without a shim.
	extra := *c
	extra.Packages = map[string]Package{}
	for name, p := range c.Packages {
		if name != "zk" {
			extra.Packages[name] = p
		}
	}
	if err := CheckOfficial(&extra, tools); err == nil {
		t.Error("accepted a tools recipe that has no shim")
	}
	// No tools directory while shims exist.
	if err := CheckOfficial(c, nil); err == nil {
		t.Error("accepted shims without tools recipes")
	}
}

const validCatalog = `{"schema":1,
"nixpkgs":{"rev":"e62746b0b472753ef019baceab6f1e2e52014980","url":"https://releases.nixos.org/nixpkgs/nixpkgs-26.11pre1104102.e62746b0b472/nixexprs.tar.zst","narHash":"sha256-GYpArmwi/iMuX2ov31oCBhAXRMJdNX9kOl2LZHCuR6M="},
"cache":{"url":"https://cache.nixos.org","public_key":"cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="},
"platforms":{"amd64":"x86_64-linux","arm64":"aarch64-linux"},
"packages":{"git":{"provider":"nix","attr":"gitMinimal"}}}`

func TestCatalogAcceptsTheMinimalFixture(t *testing.T) {
	if _, err := ParseCatalog([]byte(validCatalog)); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRejects(t *testing.T) {
	cases := map[string][2]string{
		"schema":             {`"schema":1`, `"schema":2`},
		"short rev":          {`e62746b0b472753ef019baceab6f1e2e52014980","url`, `e62746b0","url`},
		"foreign url":        {`https://releases.nixos.org/nixpkgs/`, `https://example.org/nixpkgs/`},
		"url of other rev":   {`nixpkgs-26.11pre1104102.e62746b0b472/`, `nixpkgs-26.11pre1104102.aaaaaaaaaaaa/`},
		"narHash":            {`sha256-GYpArmwi/iMuX2ov31oCBhAXRMJdNX9kOl2LZHCuR6M=`, `md5-abc`},
		"foreign cache":      {`"url":"https://cache.nixos.org"`, `"url":"https://cache.example.org"`},
		"http cache":         {`"url":"https://cache.nixos.org"`, `"url":"http://cache.nixos.org"`},
		"other key":          {`6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=`, `AAAAdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=`},
		"platform":           {`"arm64":"aarch64-linux"`, `"arm64":"x86_64-linux"`},
		"extra platform":     {`"amd64":"x86_64-linux"`, `"amd64":"x86_64-linux","riscv64":"riscv64-linux"`},
		"provider":           {`"provider":"nix"`, `"provider":"nixos"`},
		"attr injection":     {`gitMinimal`, `gitMinimal; builtins.exec`},
		"unknown field":      {`"attr":"gitMinimal"`, `"attr":"gitMinimal","url":"x"`},
		"bad name":           {`"git":{`, `"Git ":{`},
		"unknown role":       {`"attr":"gitMinimal"`, `"attr":"gitMinimal","role":"boss"`},
		"empty packages":     {`"packages":{"git":{"provider":"nix","attr":"gitMinimal"}}`, `"packages":{}`},
		"official no runner": {`"git":{"provider":"nix","attr":"gitMinimal"}`, `"hx":{"provider":"official","binary":"bin/hx"}`},
		"official bundle":    {`"git":{"provider":"nix","attr":"gitMinimal"}`, `"hx":{"provider":"official","bundle":"toolbox","binary":"bin/hx"},"glibc":{"provider":"nix","attr":"glibc","role":"runner"},"gcc-lib":{"provider":"nix","attr":"stdenv.cc.cc.lib","role":"runner"}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			text := strings.Replace(validCatalog, c[0], c[1], 1)
			if text == validCatalog {
				t.Fatal("fixture mutation did not apply")
			}
			if _, err := ParseCatalog([]byte(text)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if _, err := ParseCatalog([]byte(validCatalog + "{}")); err == nil {
		t.Error("accepted trailing data")
	}
}

func TestCatalogOfficialPathsStayInsideTheToolsDirectory(t *testing.T) {
	runners := `"glibc":{"provider":"nix","attr":"glibc","role":"runner"},"gcc-lib":{"provider":"nix","attr":"stdenv.cc.cc.lib","role":"runner"},`
	base := strings.Replace(validCatalog, `"packages":{`, `"packages":{`+runners, 1)
	for _, binary := range []string{"/bin/hx", "../hx", "bin/../../hx", "bin//hx", ""} {
		text := strings.Replace(base, `"git":`, `"hx":{"provider":"official","binary":"`+binary+`"},"git":`, 1)
		if _, err := ParseCatalog([]byte(text)); err == nil {
			t.Errorf("accepted binary %q", binary)
		}
	}
	text := strings.Replace(base, `"git":`, `"hx":{"provider":"official","binary":"bin/hx"},"git":`, 1)
	if _, err := ParseCatalog([]byte(text)); err != nil {
		t.Errorf("rejected a valid official package: %v", err)
	}
}
