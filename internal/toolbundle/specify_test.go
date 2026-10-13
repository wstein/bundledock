package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// These fixtures execute only stdlib Python and synthetic pip/venv commands.
func specifyFixture(t *testing.T) (string, string, string) { return pythonToolFixture(t, "specify") }

func pythonToolFixture(t *testing.T, tool string) (string, string, string) {
	t.Helper()
	dst, home, bin := t.TempDir(), t.TempDir(), t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writer := writeSpecifyLauncher
	if tool == "tldr" {
		writer = writeTldrLauncher
	}
	if err := writer(root); err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("fixture Python unavailable")
	}
	fixture := filepath.Join(bin, "fixture.py")
	script := `import os, pathlib, runpy, sys
args = sys.argv[1:]
home = pathlib.Path(os.environ["HOME"])
if args[:3] == ["-I", "-B", "-m"]:
    with (home / "calls").open("a") as out:
        out.write(repr(args) + "\\n")
    if args[3] == "venv":
        venv = pathlib.Path(args[-1]); (venv / "bin").mkdir(parents=True)
        (venv / "pyvenv.cfg").write_text("home = " + str(pathlib.Path(__file__).parent) + "\ninclude-system-site-packages = false\n")
        (venv / "bin/python").symlink_to(pathlib.Path(__file__).with_name("python3"))
    elif args[3] == "pip":
        assert os.environ.get("PIP_CONFIG_FILE") == "/dev/null"
        assert not any(k.startswith("PIP_") and k != "PIP_CONFIG_FILE" for k in os.environ)
        assert not any(k.startswith("PYTHON") or k.startswith("UV_") for k in os.environ)
        assert "--only-binary=:all:" in args and "--index-url" in args
        assert args[args.index("--index-url")+1] == "https://pypi.org/simple"
        if (home / "fail-install").exists(): sys.exit(7)
        venv = pathlib.Path(os.environ["PATH"].split(os.pathsep)[0]).parent
        name = "tldr" if "tldr" in str(venv) else "specify"
        tool = venv / "bin" / name
        tool.write_text('import os, sys\nassert not any(k.startswith(("PIP_", "PYTHON", "UV_")) for k in os.environ)\nprint("\\n".join(sys.argv[1:]))\nsys.exit(23)\n')
        tool.chmod(0o755)
    else: raise AssertionError(args)
elif args[:2] == ["-I", "-B"] and (args[2].endswith("/bin/specify") or args[2].endswith("/bin/tldr")):
    sys.argv = args[2:]
    runpy.run_path(sys.argv[0], run_name="__main__")
else:
    assert args[:2] == ["-I", "-B"]
    sys.executable = str(pathlib.Path(__file__).with_name("python3"))
    if (home / "preflight").exists(): exec((home / "preflight").read_text())
    sys.argv = args[2:]
    runpy.run_path(sys.argv[0], run_name="__main__")
`
	if err := os.WriteFile(fixture, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	wrapper := "#!/bin/sh\nexec '" + python + "' -I -B '" + fixture + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "python3"), []byte(wrapper), 0755); err != nil {
		t.Fatal(err)
	}
	return dst, home, bin
}

func specifyCommand(dst, home, bin string, args ...string) *exec.Cmd {
	return pythonToolCommand(dst, home, bin, "specify", args...)
}

func pythonToolCommand(dst, home, bin, tool string, args ...string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(dst, "bin/"+tool), args...)
	cmd.Env = []string{"PATH=" + bin + ":/bin", "HOME=" + home, "PIP_INDEX_URL=https://untrusted.invalid", "PIP_EXTRA_INDEX_URL=https://untrusted.invalid", "PIP_TARGET=/untrusted", "PIP_REQUIREMENT=/untrusted", "PIP_CONFIG_FILE=/untrusted", "PYTHONPATH=/untrusted", "UV_INDEX=https://untrusted.invalid"}
	return cmd
}

func TestSpecifyLauncher(t *testing.T) {
	dst, home, bin := specifyFixture(t)
	for i := 0; i < 2; i++ {
		got, err := specifyCommand(dst, home, bin, "init", "project with spaces", "--here").CombinedOutput()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 || string(got) != "init\nproject with spaces\n--here\n" {
			t.Fatalf("exit %v output %q", err, got)
		}
	}
	calls, err := os.ReadFile(filepath.Join(home, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "'venv'") != 1 || strings.Count(string(calls), "'pip'") != 1 {
		t.Fatalf("not reused: %s", calls)
	}
	if !strings.Contains(string(calls), "specify.requirements.txt") {
		t.Fatal("owned requirements not used", string(calls))
	}
	if _, err := os.Stat(filepath.Join(home, ".cache/bundledock/pip/specify")); err != nil {
		t.Fatal(err)
	}
}

func TestSpecifyFailedInstallCanRetry(t *testing.T) {
	dst, home, bin := specifyFixture(t)
	failure := filepath.Join(home, "fail-install")
	if err := os.WriteFile(failure, nil, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := specifyCommand(dst, home, bin, "--help").CombinedOutput()
	if err == nil || !strings.Contains(string(got), "wheel installation failed") {
		t.Fatalf("err %v got %s", err, got)
	}
	if err := os.Remove(failure); err != nil {
		t.Fatal(err)
	}
	got, err = specifyCommand(dst, home, bin, "--help").CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatalf("retry err %v got %s", err, got)
	}
	calls, err := os.ReadFile(filepath.Join(home, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "'venv'") != 2 {
		t.Fatal("incomplete venv reused", string(calls))
	}
}

func TestSpecifyPreflight(t *testing.T) {
	for _, tc := range []struct{ name, code, want string }{
		{"old Python", "sys.version_info=(3,10,99)", "Python 3.11+ from the language image"},
		{"virtual Python", "sys.prefix='/virtual'", "Python 3.11+ from the language image"},
		{"managed Python", "sys.executable='/home/user/.local/share/uv/python/cpython/bin/python3'", "Python 3.11+ from the language image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst, home, bin := specifyFixture(t)
			if err := os.WriteFile(filepath.Join(home, "preflight"), []byte(tc.code), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := specifyCommand(dst, home, bin, "--help").CombinedOutput()
			if err == nil || !strings.Contains(string(got), tc.want) {
				t.Fatalf("err %v got %s", err, got)
			}
			if _, err := os.Stat(filepath.Join(home, "calls")); !os.IsNotExist(err) {
				t.Fatal("installer ran during failed preflight")
			}
		})
	}
}

func TestSpecifyMetadata(t *testing.T) {
	dst, _, _ := specifyFixture(t)
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteManifest(root, "amd64", map[string]Spec{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bin/specify", "libexec/python-tool.py", "share/specify/specify.requirements.txt"} {
		if !strings.Contains(string(data), "  "+name+"\n") {
			t.Fatal("unhashed", name)
		}
	}
	if _, err := BundleLinks(root, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "share/specify/specify.requirements.txt"), []byte("specify-cli==9.9.9\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := BundleLinks(root, ""); err == nil {
		t.Fatal("accepted changed runtime requirements")
	}
}

func TestSpecifyConcurrentFirstUse(t *testing.T) {
	dst, home, bin := specifyFixture(t)
	commands := make([]*exec.Cmd, 4)
	for i := range commands {
		commands[i] = specifyCommand(dst, home, bin, "--help")
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err == nil || err.(*exec.ExitError).ExitCode() != 23 {
			t.Fatal(err)
		}
	}
	calls, err := os.ReadFile(filepath.Join(home, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "'venv'") != 1 || strings.Count(string(calls), "'pip'") != 1 {
		t.Fatal("concurrent duplicate install", string(calls))
	}
}

func TestSpecifyChangedVenvRecreatesOwnedEnvironment(t *testing.T) {
	for _, change := range []string{"configuration", "interpreter", "entrypoint"} {
		t.Run(change, func(t *testing.T) {
			dst, home, bin := specifyFixture(t)
			got, err := specifyCommand(dst, home, bin, "--help").CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
				t.Fatalf("err %v got %s", err, got)
			}
			dirs, err := filepath.Glob(filepath.Join(home, ".local/share/bundledock/tools/specify", "[a-f0-9]*"))
			if err != nil || len(dirs) != 1 {
				t.Fatal(err, dirs)
			}
			switch change {
			case "configuration":
				err = os.WriteFile(filepath.Join(dirs[0], "pyvenv.cfg"), []byte("home = /untrusted\ninclude-system-site-packages = true\n"), 0600)
			case "interpreter":
				err = os.Remove(filepath.Join(dirs[0], "bin/python"))
				if err == nil {
					err = os.Symlink("/untrusted/python", filepath.Join(dirs[0], "bin/python"))
				}
			case "entrypoint":
				err = os.WriteFile(filepath.Join(dirs[0], "bin/specify"), []byte("raise RuntimeError('UNEXPECTED')"), 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err = specifyCommand(dst, home, bin, "--help").CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
				t.Fatalf("err %v got %s", err, got)
			}
			calls, err := os.ReadFile(filepath.Join(home, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(calls), "'venv'") != 2 {
				t.Fatal("changed environment reused", string(calls))
			}
		})
	}
}

func TestSpecifyStateOutsideToolbox(t *testing.T) {
	for _, variable := range []string{"HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Run(variable, func(t *testing.T) {
			dst, home, bin := specifyFixture(t)
			cmd := specifyCommand(dst, home, bin, "--help")
			// Replace rather than duplicate HOME; getenv duplicate behavior differs by OS.
			for i, value := range cmd.Env {
				if strings.HasPrefix(value, variable+"=") {
					cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
					break
				}
			}
			cmd.Env = append(cmd.Env, variable+"="+dst)
			got, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(got), "must be outside the toolbox") {
				t.Fatalf("err %v got %s", err, got)
			}
			if _, err := os.Stat(filepath.Join(home, "calls")); !os.IsNotExist(err) {
				t.Fatal("installer executed on failed preflight")
			}
		})
	}
}

func TestSpecifyPreservesUnrecognizedState(t *testing.T) {
	for _, state := range []string{"unowned directory", "symlink"} {
		t.Run(state, func(t *testing.T) {
			dst, home, bin := specifyFixture(t)
			got, err := specifyCommand(dst, home, bin, "--help").CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
				t.Fatalf("err %v got %s", err, got)
			}
			dirs, err := filepath.Glob(filepath.Join(home, ".local/share/bundledock/tools/specify", "[a-f0-9]*"))
			if err != nil || len(dirs) != 1 {
				t.Fatal(err, dirs)
			}
			preserved := dirs[0]
			if state == "symlink" {
				preserved = t.TempDir()
				if err := os.Rename(dirs[0], dirs[0]+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(preserved, dirs[0]); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(filepath.Join(dirs[0], ".bundledock-owned")); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(preserved, "user-content")
			if err := os.WriteFile(sentinel, []byte("preserve me"), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(home, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			got, err = specifyCommand(dst, home, bin, "--help").CombinedOutput()
			if err == nil || (!strings.Contains(string(got), "unrecognized Python tool environment") && !strings.Contains(string(got), "must not be a symbolic link")) {
				t.Fatalf("err %v got %s", err, got)
			}
			after, err := os.ReadFile(filepath.Join(home, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("installer ran against unrecognized state")
			}
			data, err := os.ReadFile(sentinel)
			if err != nil || string(data) != "preserve me" {
				t.Fatalf("protected content changed: %q %v", data, err)
			}
		})
	}
}

func TestSpecifyRejectsUnsafeOwnedDirectories(t *testing.T) {
	for _, location := range []string{"state", "venv"} {
		for _, unsafe := range []string{"writable by others", "foreign owner"} {
			t.Run(location+" "+unsafe, func(t *testing.T) {
				dst, home, bin := specifyFixture(t)
				got, err := specifyCommand(dst, home, bin, "--help").CombinedOutput()
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
					t.Fatalf("err %v got %s", err, got)
				}
				state := filepath.Join(home, ".local/share/bundledock/tools/specify")
				dirs, err := filepath.Glob(filepath.Join(state, "[a-f0-9]*"))
				if err != nil || len(dirs) != 1 {
					t.Fatal(err, dirs)
				}
				target := state
				if location == "venv" {
					target = dirs[0]
				}
				sentinel := filepath.Join(target, "user-content")
				if err := os.WriteFile(sentinel, []byte("preserve me"), 0600); err != nil {
					t.Fatal(err)
				}
				if unsafe == "writable by others" {
					if err := os.Chmod(target, 0777); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { os.Chmod(target, 0700) })
				} else {
					// Simulate a different current UID without needing privileged chown.
					// stat still observes the real fixture directory owner.
					code := "original_stat=os.lstat\ntarget=" + strconv.Quote(target) + "\n" +
						"def foreign_stat(path, *args, **kwargs):\n result=original_stat(path,*args,**kwargs)\n if str(path)==target:\n  fields=list(result);fields[4]=result.st_uid+1;return os.stat_result(fields)\n return result\nos.lstat=foreign_stat\n"
					if err := os.WriteFile(filepath.Join(home, "preflight"), []byte(code), 0600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(filepath.Join(home, "calls"))
				if err != nil {
					t.Fatal(err)
				}
				got, err = specifyCommand(dst, home, bin, "--help").CombinedOutput()
				if err == nil || !strings.Contains(string(got), "must be owned by the current user and not group/other writable") {
					t.Fatalf("unsafe directory executed: err %v got %s", err, got)
				}
				after, err := os.ReadFile(filepath.Join(home, "calls"))
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatal("installer ran against unsafe owned state")
				}
				data, err := os.ReadFile(sentinel)
				if err != nil || string(data) != "preserve me" {
					t.Fatalf("protected content changed: %q %v", data, err)
				}
			})
		}
	}
}
