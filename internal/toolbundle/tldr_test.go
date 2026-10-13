package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTldrRequirements(t *testing.T) {
	data, err := os.ReadFile("python/tldr/tldr.requirements.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(data))
	if len(lines) != 3 {
		t.Fatalf("expected the complete three-wheel Linux closure, got %d", len(lines))
	}
	for _, name := range []string{"tldr-3.4.4-py3-none-any.whl", "shtab-1.12.1-py3-none-any.whl", "termcolor-3.3.0-py3-none-any.whl"} {
		if !strings.Contains(string(data), name+"#sha256=") {
			t.Fatalf("missing reviewed wheel: %s", name)
		}
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "https://files.pythonhosted.org/packages/") {
			t.Fatalf("untrusted wheel origin: %s", line)
		}
	}
}

// Shared fixture records synthetic pip/venv calls; it never installs packages.
func TestTldrLauncher(t *testing.T) {
	dst, home, bin := pythonToolFixture(t, "tldr")
	for i := 0; i < 2; i++ {
		got, err := pythonToolCommand(dst, home, bin, "tldr", "tar", "argument with spaces", "--markdown").CombinedOutput()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 || string(got) != "tar\nargument with spaces\n--markdown\n" {
			t.Fatalf("exit %v output %q", err, got)
		}
	}
	calls, err := os.ReadFile(filepath.Join(home, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "'venv'") != 1 || strings.Count(string(calls), "'pip'") != 1 {
		t.Fatalf("tool was reinstalled: %s", calls)
	}
	if !strings.Contains(string(calls), "tldr.requirements.txt") || !strings.Contains(string(calls), "--no-deps") {
		t.Fatalf("reviewed closure not enforced: %s", calls)
	}
	if _, err := os.Stat(filepath.Join(home, ".cache/bundledock/pip/tldr")); err != nil {
		t.Fatal(err)
	}
}

func TestTldrPythonMinimum(t *testing.T) {
	for _, tc := range []struct {
		version  string
		accepted bool
	}{
		{"(3, 9, 99)", false}, {"(3, 10, 0)", true},
	} {
		dst, home, bin := pythonToolFixture(t, "tldr")
		if err := os.WriteFile(filepath.Join(home, "preflight"), []byte("sys.version_info="+tc.version), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := pythonToolCommand(dst, home, bin, "tldr", "--help").CombinedOutput()
		if tc.accepted {
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
				t.Fatalf("supported Python rejected: %v %s", err, got)
			}
		} else {
			if err == nil || !strings.Contains(string(got), "Python 3.10+") {
				t.Fatalf("unsupported Python accepted: %v %s", err, got)
			}
			if _, err := os.Stat(filepath.Join(home, "calls")); !os.IsNotExist(err) {
				t.Fatal("installer ran during failed preflight")
			}
		}
	}
}

func TestTldrManifestAndNotices(t *testing.T) {
	dst, _, _ := pythonToolFixture(t, "tldr")
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteManifest(root, "amd64", map[string]Spec{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tldr": "tldr==3.4.4"`) {
		t.Fatalf("missing separate tldr launcher pin: %s", data)
	}
	sums, err := os.ReadFile(filepath.Join(dst, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bin/tldr", "libexec/python-tool.py", "share/tldr/tldr.requirements.txt", "licenses/tldr/tldr.LICENSE", "licenses/tldr/shtab.LICENSE", "licenses/tldr/termcolor.LICENSE", "licenses/tldr/SOURCE"} {
		if !strings.Contains(string(sums), "  "+name+"\n") {
			t.Fatal("unhashed tldr source input", name)
		}
	}
	if _, err := BundleLinks(root, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "share/tldr/tldr.requirements.txt"), []byte("tldr==99\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := BundleLinks(root, ""); err == nil {
		t.Fatal("accepted altered tldr wheel input")
	}
}
