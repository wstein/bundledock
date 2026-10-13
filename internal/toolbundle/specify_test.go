package toolbundle

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Only synthetic uvx and Python scripts execute; this test never installs tools.
func TestSpecifyLauncher(t *testing.T) {
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	p := testPackages(t)["uv"]
	if err := Extract(root, "uv", p, bytes.NewReader(archive(t, member{"uv", "uv", 0755, 0}, member{"uvx", "#!/bin/sh\ntest -z \"${UV_INDEX+x}${UV_DEFAULT_INDEX+x}${UV_INDEX_URL+x}${UV_EXTRA_INDEX_URL+x}${UV_FIND_LINKS+x}${PIP_INDEX_URL+x}${PIP_EXTRA_INDEX_URL+x}${PIP_FIND_LINKS+x}${UV_OVERRIDE+x}${UV_CONSTRAINT+x}${UV_WORKING_DIR+x}\" || exit 99\nprintf '%s\\n' \"$UV_PYTHON_DOWNLOADS\" \"$UV_PYTHON_PREFERENCE\" \"$UV_CACHE_DIR\" \"$@\"\nexit 23\n", 0755, 0}))); err != nil {
		t.Fatal(err)
	}
	pythonBin := t.TempDir()
	python := filepath.Join(pythonBin, "python3")
	if err := os.WriteFile(python, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	cache := filepath.Join(home, "cache with spaces")
	cmd := exec.Command(filepath.Join(dst, "bin/specify"), "init", "project with spaces", "--here")
	cmd.Dir = pythonBin
	cmd.Env = []string{"PATH=.:/bin", "HOME=" + home, "UV_CACHE_DIR=" + cache, "UV_INDEX=https://untrusted.invalid/simple", "UV_DEFAULT_INDEX=https://untrusted.invalid/simple", "UV_INDEX_URL=https://untrusted.invalid/simple", "UV_EXTRA_INDEX_URL=https://untrusted.invalid/simple", "UV_FIND_LINKS=/untrusted/wheels", "PIP_INDEX_URL=https://untrusted.invalid/simple", "PIP_EXTRA_INDEX_URL=https://untrusted.invalid/simple", "PIP_FIND_LINKS=/untrusted/wheels", "UV_OVERRIDE=/untrusted/override.txt", "UV_CONSTRAINT=/untrusted/constraint.txt", "UV_WORKING_DIR=/untrusted/project"}
	got, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatalf("exit: %v output %q", err, got)
	}
	want := "never\nonly-system\n" + cache + "\n--no-config\n--no-managed-python\n--no-python-downloads\n--no-build\n--default-index\nhttps://pypi.org/simple\n--python\n" + python + "\n--from\nspecify-cli==1.1.3\nspecify\ninit\nproject with spaces\n--here\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("writable cache unavailable: %v", err)
	}
	for _, env := range []struct{ extra, cache string }{
		{"", filepath.Join(home, ".cache/uv")},
		{"UV_CACHE_DIR=relative cache", filepath.Join(pythonBin, "relative cache")},
		{"XDG_CACHE_HOME=" + filepath.Join(home, "xdg cache"), filepath.Join(home, "xdg cache/uv")},
	} {
		cmd := exec.Command(filepath.Join(dst, "bin/specify"), "--help")
		cmd.Dir = pythonBin
		cmd.Env = []string{"PATH=" + pythonBin + ":/bin", "HOME=" + home}
		if env.extra != "" {
			cmd.Env = append(cmd.Env, env.extra)
		}
		got, err := cmd.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 || !strings.Contains(string(got), "\n"+env.cache+"\n") {
			t.Fatalf("default cache: %v %q", err, got)
		}
	}
	if err := WriteManifest(root, "amd64", map[string]Spec{}); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(dst, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"specify": "specify-cli==1.1.3"`) {
		t.Fatal("runtime launcher pin absent from manifest")
	}
	sums, err := os.ReadFile(filepath.Join(dst, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sums), "  bin/specify\n") {
		t.Fatal("launcher absent from checksums")
	}
	if _, err := BundleLinks(root, ""); err != nil {
		t.Fatalf("read own manifest: %v", err)
	}
	bad := strings.Replace(string(manifest), "specify-cli==1.1.3", "specify-cli==9.9.9", 1)
	if err := os.WriteFile(filepath.Join(dst, "manifest.json"), []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := BundleLinks(root, ""); err == nil || !strings.Contains(err.Error(), "invalid runtime launcher metadata") {
		t.Fatalf("accepted invalid launcher metadata: %v", err)
	}
}

func TestSpecifyPreflight(t *testing.T) {
	for _, tc := range []struct{ name, python, home, cache, uvx, want string }{
		{"missing python", "missing", "normal", "normal", "present", "Python 3.11+ is required"},
		{"old python", "sys.version_info=(3,10,99)", "normal", "normal", "present", "Python 3.11+ from the language image"},
		{"virtual python", "sys.prefix='/virtual'", "normal", "normal", "present", "Python 3.11+ from the language image"},
		{"managed python", "sys.executable='/home/user/.local/share/uv/python/cpython/bin/python3'", "normal", "normal", "present", "Python 3.11+ from the language image"},
		{"custom managed python", "sys.executable='/custom-managed/cpython/bin/python3'", "normal", "normal", "present", "Python 3.11+ from the language image"},
		{"missing uvx", "exit 0", "normal", "normal", "missing", "uvx executable is missing"},
		{"unset home", "exit 0", "unset", "normal", "present", "HOME must be a writable"},
		{"invalid home", "exit 0", "invalid", "normal", "present", "HOME must be a writable"},
		{"toolbox home", "exit 0", "bundle", "normal", "present", "HOME must be outside"},
		{"toolbox cache", "exit 0", "normal", "bundle", "present", "UV_CACHE_DIR must be outside"},
		{"invalid cache", "exit 0", "normal", "invalid", "present", "UV_CACHE_DIR must be writable"},
		{"symlink cache", "exit 0", "normal", "symlink", "present", "UV_CACHE_DIR must be outside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := t.TempDir()
			root, err := os.OpenRoot(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := writeSpecifyLauncher(root); err != nil {
				t.Fatal(err)
			}
			if tc.uvx == "present" {
				if err := WriteFile(root, "bin/uvx", strings.NewReader("#!/bin/sh\necho UNEXPECTED_UVX\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			bin := t.TempDir()
			if strings.HasPrefix(tc.python, "sys.") {
				systemPython, err := exec.LookPath("python3")
				if err != nil {
					t.Skip("Python fixture interpreter unavailable")
				}
				tc.python = "exec '" + systemPython + "' -I -B -c 'import sys; " + strings.ReplaceAll(tc.python, "'", `'\''`) + "; exec(sys.argv[1])' \"$4\""
			}
			if tc.python != "missing" {
				if err := os.WriteFile(filepath.Join(bin, "python3"), []byte("#!/bin/sh\n"+tc.python+"\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			// Limit PATH to fixtures; only harmless system mkdir is needed by preflight.
			if err := os.Symlink("/bin/mkdir", filepath.Join(bin, "mkdir")); err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			switch tc.home {
			case "unset":
				home = ""
			case "invalid":
				home = filepath.Join(home, "missing")
			case "bundle":
				home = dst
			}
			cache := filepath.Join(t.TempDir(), "cache")
			switch tc.cache {
			case "bundle":
				cache = dst
			case "invalid":
				cache = filepath.Join(dst, "bin/specify/child")
			case "symlink":
				cache = filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(dst, cache); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(filepath.Join(dst, "bin/specify"), "--help")
			cmd.Env = []string{"PATH=" + bin, "HOME=" + home, "UV_CACHE_DIR=" + cache, "UV_INDEX=https://untrusted.invalid/simple", "UV_DEFAULT_INDEX=https://untrusted.invalid/simple", "UV_INDEX_URL=https://untrusted.invalid/simple", "UV_EXTRA_INDEX_URL=https://untrusted.invalid/simple", "UV_FIND_LINKS=/untrusted/wheels", "PIP_INDEX_URL=https://untrusted.invalid/simple", "PIP_EXTRA_INDEX_URL=https://untrusted.invalid/simple", "PIP_FIND_LINKS=/untrusted/wheels", "UV_OVERRIDE=/untrusted/override.txt", "UV_CONSTRAINT=/untrusted/constraint.txt", "UV_WORKING_DIR=/untrusted/project"}
			if tc.name == "custom managed python" {
				cmd.Env = append(cmd.Env, "UV_PYTHON_INSTALL_DIR=/custom-managed")
			}
			got, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(got), tc.want) || strings.Contains(string(got), "UNEXPECTED_UVX") {
				t.Fatalf("err=%v output=%q want=%q", err, got, tc.want)
			}
		})
	}
}
