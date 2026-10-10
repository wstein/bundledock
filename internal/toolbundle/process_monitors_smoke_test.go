package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Set BUNDLEDOCK_PROCESS_MONITORS_SMOKE_PATH to a PATH containing the two real executables.
// OCI smoke runs the same script without network as an unprivileged user.
func TestProcessMonitorsOfflineSmoke(t *testing.T) {
	toolPath := os.Getenv("BUNDLEDOCK_PROCESS_MONITORS_SMOKE_PATH")
	if toolPath == "" {
		t.Skip("real process monitor binaries not supplied")
	}
	script, err := filepath.Abs("../../bundles/toolbox/process-monitors-smoke.sh")
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
