package toolbundle

import (
	"os"
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
