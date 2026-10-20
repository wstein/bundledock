package shelllayer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gsyncEnv is a hermetic environment: no user or system Git configuration, a
// fixed identity through the environment (never written to a config file), and
// no repository inherited from the caller.
func gsyncEnv(home, bin string) []string {
	return []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home,
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig"), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=protocol.file.allow", "GIT_CONFIG_VALUE_0=always",
		"GIT_CONFIG_KEY_1=init.defaultBranch", "GIT_CONFIG_VALUE_1=main",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CEILING_DIRECTORIES=" + filepath.Dir(home),
		"TERM=dumb", "BDOCK_STARSHIP=0", "BDOCK_FASTFETCH=0", "BDOCK_ZOXIDE=0", "LC_ALL=C",
	}
}

type gsyncFixture struct {
	t    *testing.T
	root string
	home string
	bin  string
	log  string
}

// newGsyncFixture puts a logging git wrapper first on PATH: it records every
// argv and then runs the real git, so a test can prove which verbs ran.
func newGsyncFixture(t *testing.T) *gsyncFixture {
	t.Helper()
	root := t.TempDir()
	f := &gsyncFixture{t: t, root: root, home: filepath.Join(root, "home"), bin: filepath.Join(root, "bin"), log: filepath.Join(root, "git.log")}
	for _, d := range []string{f.home, f.bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	wrapper := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + f.log + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(f.bin, "git"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *gsyncFixture) path(parts ...string) string {
	return filepath.Join(append([]string{f.root}, parts...)...)
}

// git runs the real git (not logged) in dir and returns trimmed stdout.
func (f *gsyncFixture) git(dir string, args ...string) string {
	f.t.Helper()
	out, err := f.gitErr(dir, args...)
	if err != nil {
		f.t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return out
}

func (f *gsyncFixture) gitErr(dir string, args ...string) (string, error) {
	real, _ := exec.LookPath("git")
	cmd := exec.Command(real, args...)
	cmd.Dir = dir
	cmd.Env = gsyncEnv(f.home, f.bin)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (f *gsyncFixture) write(dir, name, text string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *gsyncFixture) commit(dir, name, text, msg string) {
	f.t.Helper()
	f.write(dir, name, text)
	f.git(dir, "add", name)
	f.git(dir, "commit", "-q", "-m", msg)
}

// pair creates a bare origin, a seed clone that publishes main, and a second
// clone named "clone" that tracks it.
func (f *gsyncFixture) pair() (origin, seed, clone string) {
	f.t.Helper()
	origin, seed, clone = f.path("origin.git"), f.path("seed"), f.path("clone")
	f.git(f.root, "init", "-q", "--bare", origin)
	f.git(f.root, "clone", "-q", origin, seed)
	f.commit(seed, "a.txt", "one\n", "one")
	f.git(seed, "push", "-q", "origin", "main")
	f.git(f.root, "clone", "-q", origin, clone)
	return origin, seed, clone
}

type gsyncShell struct {
	name string
	argv func(file, command string) []string
}

var gsyncShells = []gsyncShell{
	{"bash", func(file, c string) []string {
		return []string{"bash", "--norc", "--noprofile", "-i", "-c", "source " + file + "; " + c}
	}},
	{"zsh", func(file, c string) []string { return []string{"zsh", "-f", "-i", "-c", "source " + file + "; " + c} }},
	{"fish", func(file, c string) []string {
		return []string{"fish", "--no-config", "--interactive", "-c", "source " + file + "; " + c}
	}},
	{"nu", func(file, c string) []string {
		return []string{"nu", "--no-config-file", "--no-history", "-c", "source " + file + "; " + c}
	}},
}

var gsyncFiles = map[string]string{"bash": BashPath, "zsh": ZshPath, "fish": FishPath, "nu": NuPath}

// eachShell runs fn once per installed shell with a runner for the shortcut.
func eachShell(t *testing.T, fn func(t *testing.T, sh gsyncShell, run func(f *gsyncFixture, dir, command string) (string, int))) {
	layer := layerDir(t)
	for _, sh := range gsyncShells {
		sh := sh
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.name); err != nil {
				t.Skipf("%s not installed: gsync NOT verified", sh.name)
			}
			run := func(f *gsyncFixture, dir, command string) (string, int) {
				t.Helper()
				argv := sh.argv(filepath.Join(layer, gsyncFiles[sh.name]), command)
				cmd := exec.Command(argv[0], argv[1:]...)
				cmd.Dir = dir
				cmd.Env = gsyncEnv(f.home, f.bin)
				out, err := cmd.CombinedOutput()
				code := 0
				if err != nil {
					ee, ok := err.(*exec.ExitError)
					if !ok {
						t.Fatalf("%s: %v", argv[0], err)
					}
					code = ee.ExitCode()
					if code == 0 {
						code = -1
					}
				}
				return string(out), code
			}
			fn(t, sh, run)
		})
	}
}

type gsyncRun func(f *gsyncFixture, dir, command string) (string, int)

func TestGsyncBehindByN(t *testing.T) {
	eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
		f := newGsyncFixture(t)
		_, seed, clone := f.pair()
		f.commit(seed, "b.txt", "b\n", "two")
		f.commit(seed, "c.txt", "c\n", "three")
		f.git(seed, "push", "-q", "origin", "main")
		out, code := run(f, clone, "gsync")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if got, want := f.git(clone, "rev-parse", "HEAD"), f.git(seed, "rev-parse", "HEAD"); got != want {
			t.Errorf("HEAD %s, want %s\n%s", got, want, out)
		}
		logged, _ := os.ReadFile(f.log)
		if !strings.Contains(string(logged), "fetch --prune --all") || !strings.Contains(string(logged), "pull --rebase --autostash") {
			t.Errorf("expected explicit fetch --prune --all and pull --rebase --autostash, got:\n%s", logged)
		}
	})
}

func TestGsyncAfterRebaseMergeAndPrune(t *testing.T) {
	eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
		f := newGsyncFixture(t)
		origin, seed, clone := f.pair()
		f.git(seed, "push", "-q", "origin", "main:topic")
		f.git(clone, "fetch", "-q", "--all")
		if f.git(clone, "branch", "-r") == "" || !strings.Contains(f.git(clone, "branch", "-r"), "origin/topic") {
			t.Fatal("fixture: origin/topic missing")
		}
		// The clone has a local commit L; the owner rebase-merges the same change
		// upstream on top of a new commit, so main holds L' with another SHA.
		f.commit(clone, "l.txt", "l\n", "local change")
		f.commit(seed, "m.txt", "m\n", "upstream")
		f.commit(seed, "l.txt", "l\n", "local change")
		f.git(seed, "push", "-q", "origin", "main")
		f.git(origin, "branch", "-q", "-D", "topic")
		out, code := run(f, clone, "gsync")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if got, want := f.git(clone, "rev-parse", "HEAD"), f.git(seed, "rev-parse", "HEAD"); got != want {
			t.Errorf("HEAD %s, want rewritten upstream %s\n%s", got, want, out)
		}
		if strings.Contains(f.git(clone, "branch", "-r"), "origin/topic") {
			t.Errorf("deleted remote branch was not pruned:\n%s", f.git(clone, "branch", "-r"))
		}
	})
}

func TestGsyncAutostashesDirtyTree(t *testing.T) {
	eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
		f := newGsyncFixture(t)
		_, seed, clone := f.pair()
		f.commit(seed, "b.txt", "b\n", "two")
		f.git(seed, "push", "-q", "origin", "main")
		f.write(clone, "a.txt", "dirty\n")
		out, code := run(f, clone, "gsync")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if got, want := f.git(clone, "rev-parse", "HEAD"), f.git(seed, "rev-parse", "HEAD"); got != want {
			t.Errorf("HEAD %s, want %s", got, want)
		}
		if b, _ := os.ReadFile(filepath.Join(clone, "a.txt")); string(b) != "dirty\n" {
			t.Errorf("local change lost: %q", b)
		}
	})
}

func TestGsyncRefusesInProgressOperations(t *testing.T) {
	cases := []struct {
		name  string
		start func(f *gsyncFixture, seed, clone string)
		want  string
	}{
		{"rebase", func(f *gsyncFixture, seed, clone string) {
			f.commit(clone, "a.txt", "mine\n", "mine")
			f.commit(seed, "a.txt", "theirs\n", "theirs")
			f.git(seed, "push", "-q", "origin", "main")
			f.git(clone, "fetch", "-q")
			if _, err := f.gitErr(clone, "rebase", "origin/main"); err == nil {
				f.t.Fatal("fixture: expected a rebase conflict")
			}
		}, "rebase in progress"},
		{"merge", func(f *gsyncFixture, seed, clone string) {
			f.commit(clone, "a.txt", "mine\n", "mine")
			f.commit(seed, "a.txt", "theirs\n", "theirs")
			f.git(seed, "push", "-q", "origin", "main")
			f.git(clone, "fetch", "-q")
			if _, err := f.gitErr(clone, "merge", "origin/main"); err == nil {
				f.t.Fatal("fixture: expected a merge conflict")
			}
		}, "merge in progress"},
		{"cherry-pick", func(f *gsyncFixture, seed, clone string) {
			f.git(clone, "switch", "-q", "-c", "side")
			f.commit(clone, "a.txt", "side\n", "side")
			f.git(clone, "switch", "-q", "main")
			f.commit(clone, "a.txt", "main\n", "main")
			if _, err := f.gitErr(clone, "cherry-pick", "side"); err == nil {
				f.t.Fatal("fixture: expected a cherry-pick conflict")
			}
		}, "cherry-pick in progress"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
				f := newGsyncFixture(t)
				_, seed, clone := f.pair()
				c.start(f, seed, clone)
				before := f.git(clone, "status", "--porcelain=v2", "--branch")
				_ = os.Remove(f.log)
				out, code := run(f, clone, "gsync")
				if code == 0 || !strings.Contains(out, c.want) {
					t.Fatalf("want refusal %q, got exit %d\n%s", c.want, code, out)
				}
				if logged, _ := os.ReadFile(f.log); strings.Contains(string(logged), "pull") || strings.Contains(string(logged), "fetch") {
					t.Errorf("git ran despite the in-progress operation:\n%s", logged)
				}
				if after := f.git(clone, "status", "--porcelain=v2", "--branch"); after != before {
					t.Errorf("state changed:\n%s\n--\n%s", before, after)
				}
			})
		})
	}
}

func TestGsyncDetachedHeadSaysSo(t *testing.T) {
	eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
		f := newGsyncFixture(t)
		_, _, clone := f.pair()
		f.git(clone, "checkout", "-q", "--detach")
		head := f.git(clone, "rev-parse", "HEAD")
		out, code := run(f, clone, "gsync")
		if code == 0 || !strings.Contains(out, "detached HEAD") {
			t.Fatalf("want a detached HEAD message and a non-zero exit, got %d\n%s", code, out)
		}
		if f.git(clone, "rev-parse", "HEAD") != head {
			t.Error("HEAD moved")
		}
		if logged, _ := os.ReadFile(f.log); strings.Contains(string(logged), "pull") {
			t.Errorf("pull ran on a detached HEAD:\n%s", logged)
		}
	})
}

func TestGsyncNotAWorkTree(t *testing.T) {
	eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
		f := newGsyncFixture(t)
		plain := f.path("plain")
		if err := os.MkdirAll(plain, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"gsync", "gsync-all"} {
			out, code := run(f, plain, name)
			if code == 0 || !strings.Contains(out, "not inside a Git work tree") {
				t.Errorf("%s: want a not-a-work-tree message and a non-zero exit, got %d\n%s", name, code, out)
			}
		}
		if logged, _ := os.ReadFile(f.log); strings.Contains(string(logged), "fetch") || strings.Contains(string(logged), "pull") {
			t.Errorf("network verbs ran outside a work tree:\n%s", logged)
		}
	})
}

// umbrella builds an umbrella repository with one submodule "sub", both
// published to bare origins, and returns the umbrella clone and the submodule
// seed (a second clone of the submodule's origin, used to move it ahead).
func (f *gsyncFixture) umbrella() (root, subSeed string) {
	f.t.Helper()
	subOrigin, subSeed := f.path("sub.git"), f.path("sub-seed")
	f.git(f.root, "init", "-q", "--bare", subOrigin)
	f.git(f.root, "clone", "-q", subOrigin, subSeed)
	f.commit(subSeed, "s.txt", "s\n", "sub one")
	f.git(subSeed, "push", "-q", "origin", "main")
	umbOrigin, umbSeed := f.path("umb.git"), f.path("umb-seed")
	f.git(f.root, "init", "-q", "--bare", umbOrigin)
	f.git(f.root, "clone", "-q", umbOrigin, umbSeed)
	f.commit(umbSeed, "u.txt", "u\n", "umbrella one")
	f.git(umbSeed, "submodule", "add", "-q", subOrigin, "sub")
	f.git(umbSeed, "commit", "-q", "-m", "add sub")
	f.git(umbSeed, "push", "-q", "origin", "main")
	root = f.path("umb")
	f.git(f.root, "clone", "-q", "--recurse-submodules", umbOrigin, root)
	// A submodule checkout is detached; put it on its branch like the owner's.
	f.git(filepath.Join(root, "sub"), "switch", "-q", "main")
	return root, subSeed
}

func TestGsyncAllReportsAndOnlyFetches(t *testing.T) {
	eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
		f := newGsyncFixture(t)
		root, subSeed := f.umbrella()
		f.commit(subSeed, "s2.txt", "s2\n", "sub two")
		f.git(subSeed, "push", "-q", "origin", "main")
		f.write(root, "u.txt", "dirty\n")
		subHead := f.git(filepath.Join(root, "sub"), "rev-parse", "HEAD")
		gitlink := f.git(root, "ls-tree", "HEAD", "sub")
		_ = os.Remove(f.log)

		out, code := run(f, root, "gsync-all")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		for _, want := range []string{
			".: main, ahead 0/behind 0 of origin/main, dirty",
			"sub: main, ahead 0/behind 1 of origin/main, clean",
			"git -C sub pull --rebase --autostash",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "git -C . pull") {
			t.Errorf("umbrella is not behind, no command expected:\n%s", out)
		}
		if f.git(filepath.Join(root, "sub"), "rev-parse", "HEAD") != subHead {
			t.Error("submodule HEAD moved")
		}
		if f.git(root, "ls-tree", "HEAD", "sub") != gitlink {
			t.Error("gitlink changed")
		}
		if got, want := f.git(filepath.Join(root, "sub"), "rev-parse", "origin/main"), f.git(subSeed, "rev-parse", "HEAD"); got != want {
			t.Errorf("submodule was not fetched: origin/main %s, want %s", got, want)
		}
		logged, _ := os.ReadFile(f.log)
		fetches := 0
		for _, line := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
			for _, word := range strings.Fields(line) {
				switch word {
				case "pull", "rebase", "checkout", "switch", "reset", "merge", "stash", "clean", "restore", "submodule", "push", "cherry-pick", "commit", "add":
					t.Errorf("forbidden git verb %q ran: git %s", word, line)
				}
			}
			if strings.Contains(" "+line+" ", " fetch ") {
				fetches++
				if !strings.Contains(line, "--prune") || strings.Contains(line, "--all") {
					t.Errorf("fetch must be exactly `fetch --prune`: git %s", line)
				}
			}
		}
		if fetches != 2 {
			t.Errorf("want one fetch per repository (2), got %d:\n%s", fetches, logged)
		}
	})
}

func TestGsyncAllMarksUninitializedSubmodule(t *testing.T) {
	eachShell(t, func(t *testing.T, _ gsyncShell, run func(*gsyncFixture, string, string) (string, int)) {
		f := newGsyncFixture(t)
		root, _ := f.umbrella()
		f.git(root, "submodule", "deinit", "-q", "-f", "sub")
		out, code := run(f, root, "gsync-all")
		if code != 0 || !strings.Contains(out, "sub: not checked out") {
			t.Errorf("want a not-checked-out line and exit 0, got %d\n%s", code, out)
		}
	})
}

// The script is embedded in single quotes (and a Nushell raw string), so it
// must not hold a single quote or a backslash; its names must not collide with
// a table shortcut or a Nushell built-in.
func TestGsyncScriptIsEmbeddable(t *testing.T) {
	if strings.ContainsAny(GsyncScript(), "'\\") {
		t.Error("gsync.sh must contain no single quote and no backslash")
	}
	shortcuts, err := Shortcuts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shortcuts {
		for _, n := range GsyncNames {
			if s.Name == n {
				t.Errorf("%s collides with a table shortcut", n)
			}
		}
	}
	for _, n := range GsyncNames {
		if nushellBuiltins[n] {
			t.Errorf("%s shadows a Nushell built-in", n)
		}
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		if out, err := exec.Command(sh, "-n", filepath.Join("gsync.sh")).CombinedOutput(); err != nil {
			t.Errorf("sh -n gsync.sh: %v\n%s", err, out)
		}
	}
}
