package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Set BUNDLEDOCK_SMOKE_PATH to a PATH containing the five real executables.
// OCI smoke runs the same script without network as an unprivileged user.
func TestQualitySecurityOfflineSmoke(t *testing.T) {
	toolPath := os.Getenv("BUNDLEDOCK_SMOKE_PATH")
	if toolPath == "" {
		t.Skip("real quality/security binaries not supplied")
	}
	script, err := filepath.Abs("../../bundles/toolbox/scripts/quality-security-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	cmd := exec.Command("sh", script)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+toolPath+":"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("offline smoke: %v\n%s", err, output)
	}
}
