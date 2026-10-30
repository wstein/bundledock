package nixbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	gitPath   = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-git-2"
	sshPath   = "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-openssh-1"
	glibcPath = "/nix/store/cccccccccccccccccccccccccccccccc-glibc-2.44"
	gccPath   = "/nix/store/dddddddddddddddddddddddddddddddd-gcc-lib"
)

const profileCatalog = `{"schema":1,
"nixpkgs":{"rev":"e62746b0b472753ef019baceab6f1e2e52014980","url":"https://releases.nixos.org/nixpkgs/nixpkgs-26.11pre1104102.e62746b0b472/nixexprs.tar.zst","narHash":"sha256-GYpArmwi/iMuX2ov31oCBhAXRMJdNX9kOl2LZHCuR6M="},
"cache":{"url":"https://cache.nixos.org","public_key":"cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="},
"platforms":{"amd64":"x86_64-linux","arm64":"aarch64-linux"},
"packages":{
 "git":{"provider":"nix","attr":"gitMinimal"},
 "openssh":{"provider":"nix","attr":"openssh"},
 "glibc":{"provider":"nix","attr":"glibc","role":"runner"},
 "gcc-lib":{"provider":"nix","attr":"stdenv.cc.cc.lib","role":"runner"},
 "hx":{"provider":"official","binary":"lib/helix/hx","env":{"HELIX_RUNTIME":"lib/helix/runtime"}},
 "zk":{"provider":"official","binary":"bin/zk"}}}`

// profileFixture lays out a physical store and a lock-shaped PlatformLock for it.
func profileFixture(t *testing.T) (*Catalog, PlatformLock, string) {
	t.Helper()
	c, err := ParseCatalog([]byte(profileCatalog))
	if err != nil {
		t.Fatal(err)
	}
	store := t.TempDir()
	put := func(rel, body string, mode os.FileMode) {
		full := filepath.Join(store, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	put(filepath.Base(gitPath)+"/bin/git", "git", 0o755)
	put(filepath.Base(gitPath)+"/share/man/man1/git.1", "m", 0o644)
	put(filepath.Base(sshPath)+"/bin/ssh", "ssh", 0o755)
	put(filepath.Base(sshPath)+"/share/man/man1/ssh.1", "m", 0o644)
	put(filepath.Base(sshPath)+"/etc/ssh/ssh_config", "c", 0o644)
	put(filepath.Base(glibcPath)+"/lib/ld-linux-aarch64.so.1", "ld", 0o755)
	put(filepath.Base(glibcPath)+"/lib/ld-linux-x86-64.so.2", "ld", 0o755)
	put(filepath.Base(glibcPath)+"/bin/ldd", "ldd", 0o755)
	put(filepath.Base(gccPath)+"/lib/libgcc_s.so.1", "gcc", 0o644)
	pl := PlatformLock{System: "aarch64-linux", Roots: map[string]string{"git": gitPath, "openssh": sshPath, "glibc": glibcPath, "gcc-lib": gccPath}}
	return c, pl, store
}

// placeOfficial installs the official binaries the way the image build does,
// before the profile is built: the tools bundle goes to <profile>/libexec/nixbox.
func placeOfficial(t *testing.T, out string, binaries ...string) {
	t.Helper()
	placeBinaries(t, out, binaries...)
	if err := os.MkdirAll(filepath.Join(out, "libexec", "nixbox", "lib", "helix", "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// placeBinaries installs the binaries only, without the Helix runtime directory.
func placeBinaries(t *testing.T, out string, binaries ...string) {
	t.Helper()
	for _, rel := range binaries {
		full := filepath.Join(out, "libexec", "nixbox", rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("elf"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildProfile(t *testing.T) {
	c, pl, store := profileFixture(t)
	out := filepath.Join(t.TempDir(), "profile")
	placeOfficial(t, out, "lib/helix/hx", "bin/zk")
	shim := []byte("#!/bin/sh\n# shim\n")
	if err := BuildProfile(c, []byte(profileCatalog), []byte(`{"lock":1}`), pl, "arm64", store, out, shim); err != nil {
		t.Fatal(err)
	}
	link := func(rel string) string {
		t.Helper()
		target, err := os.Readlink(filepath.Join(out, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return target
	}
	cases := map[string]string{
		"bin/git":              gitPath + "/bin/git",
		"bin/ssh":              sshPath + "/bin/ssh",
		"share/man/man1/git.1": gitPath + "/share/man/man1/git.1",
		"share/man/man1/ssh.1": sshPath + "/share/man/man1/ssh.1",
		"etc/ssh/ssh_config":   sshPath + "/etc/ssh/ssh_config",
		"lib/ld.so":            glibcPath + "/lib/ld-linux-aarch64.so.1",
		"lib/glibc":            glibcPath + "/lib",
		"lib/gcc":              gccPath + "/lib",
		"bin/zk":               "../libexec/nixbox-shim",
		"bin/hx":               "../libexec/nixbox-shim",
	}
	for rel, want := range cases {
		if got := link(rel); got != want {
			t.Errorf("%s -> %s, want %s", rel, got, want)
		}
	}
	// Runner packages contribute no commands.
	if _, err := os.Lstat(filepath.Join(out, "bin/ldd")); err == nil {
		t.Error("glibc's ldd leaked into the profile")
	}
	// share/man is a real directory merged from two packages.
	if info, err := os.Lstat(filepath.Join(out, "share/man/man1")); err != nil || !info.IsDir() {
		t.Error("share/man/man1 must be a merged directory")
	}
	got, err := os.ReadFile(filepath.Join(out, "libexec/nixbox-shim"))
	if err != nil || string(got) != string(shim) {
		t.Errorf("shim not installed: %v %q", err, got)
	}
	if info, _ := os.Stat(filepath.Join(out, "libexec/nixbox-shim")); info.Mode().Perm() != 0o755 {
		t.Errorf("shim mode %v", info.Mode())
	}
	table, _ := os.ReadFile(filepath.Join(out, "share/nixbox/shims.tsv"))
	wantTable := "bin\thx\tlib/helix/hx\nenv\thx\tHELIX_RUNTIME\tlib/helix/runtime\nbin\tzk\tbin/zk\n"
	if string(table) != wantTable {
		t.Errorf("table = %q, want %q", table, wantTable)
	}
	for _, name := range []string{"catalog.json", "lock.json"} {
		if _, err := os.Stat(filepath.Join(out, "share/nixbox", name)); err != nil {
			t.Error(err)
		}
	}
}

func TestBuildProfileSelectsTheLoaderByArchitecture(t *testing.T) {
	c, pl, store := profileFixture(t)
	pl.System = "x86_64-linux"
	out := filepath.Join(t.TempDir(), "profile")
	placeOfficial(t, out, "lib/helix/hx", "bin/zk")
	if err := BuildProfile(c, []byte(profileCatalog), nil, pl, "amd64", store, out, []byte("#!/bin/sh\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(out, "lib/ld.so")); !strings.HasSuffix(got, "/lib/ld-linux-x86-64.so.2") {
		t.Errorf("ld.so -> %s", got)
	}
}

func TestBuildProfileRefusesCommandConflictsAndMissingLoaders(t *testing.T) {
	c, pl, store := profileFixture(t)
	if err := os.WriteFile(filepath.Join(store, filepath.Base(sshPath), "bin", "git"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "p")
	placeOfficial(t, out, "lib/helix/hx", "bin/zk")
	if err := BuildProfile(c, []byte(profileCatalog), nil, pl, "arm64", store, out, []byte("#!/bin/sh\n")); err == nil {
		t.Error("accepted two packages providing bin/git")
	}
	c, pl, store = profileFixture(t)
	os.Remove(filepath.Join(store, filepath.Base(glibcPath), "lib", "ld-linux-aarch64.so.1"))
	out = filepath.Join(t.TempDir(), "p")
	placeOfficial(t, out, "lib/helix/hx", "bin/zk")
	if err := BuildProfile(c, []byte(profileCatalog), nil, pl, "arm64", store, out, []byte("#!/bin/sh\n")); err == nil {
		t.Error("accepted a glibc without the loader")
	}
}

func TestBuildProfileRefusesAShimNameThatAPackageAlreadyProvides(t *testing.T) {
	c, pl, store := profileFixture(t)
	if err := os.WriteFile(filepath.Join(store, filepath.Base(gitPath), "bin", "zk"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "p")
	placeOfficial(t, out, "lib/helix/hx", "bin/zk")
	if err := BuildProfile(c, []byte(profileCatalog), nil, pl, "arm64", store, out, []byte("#!/bin/sh\n")); err == nil {
		t.Error("accepted a shim shadowing a nix command")
	}
}

func TestBuildProfileRefusesAShimWithoutItsInstalledBinary(t *testing.T) {
	for name, binaries := range map[string][]string{
		"none":         nil,
		"hx":           {"bin/zk"},
		"zk":           {"lib/helix/hx"},
		"not a binary": {"lib/helix/hx", "bin/zk/"},
	} {
		t.Run(name, func(t *testing.T) {
			c, pl, store := profileFixture(t)
			out := filepath.Join(t.TempDir(), "p")
			if name == "not a binary" {
				if err := os.MkdirAll(filepath.Join(out, "libexec", "nixbox", "bin", "zk"), 0o755); err != nil {
					t.Fatal(err)
				}
				binaries = []string{"lib/helix/hx"}
			}
			placeOfficial(t, out, binaries...)
			if err := BuildProfile(c, []byte(profileCatalog), nil, pl, "arm64", store, out, []byte("#!/bin/sh\n")); err == nil {
				t.Error("accepted a shim whose binary is not installed")
			}
		})
	}
}

func TestBuildProfileRefusesAMissingEnvironmentTarget(t *testing.T) {
	c, pl, store := profileFixture(t)
	out := filepath.Join(t.TempDir(), "p")
	placeBinaries(t, out, "lib/helix/hx", "bin/zk")
	if err := BuildProfile(c, []byte(profileCatalog), nil, pl, "arm64", store, out, []byte("#!/bin/sh\n")); err == nil || !strings.Contains(err.Error(), "HELIX_RUNTIME") {
		t.Errorf("accepted a missing HELIX_RUNTIME directory: %v", err)
	}
}

func TestBuildProfileRefusesANonExecutableBinary(t *testing.T) {
	c, pl, store := profileFixture(t)
	out := filepath.Join(t.TempDir(), "p")
	placeOfficial(t, out, "lib/helix/hx", "bin/zk")
	if err := os.Chmod(filepath.Join(out, "libexec", "nixbox", "bin", "zk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := BuildProfile(c, []byte(profileCatalog), nil, pl, "arm64", store, out, []byte("#!/bin/sh\n")); err == nil {
		t.Error("accepted a non-executable binary")
	}
}
