package toolbundle

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ReadPythonLock accepts exactly one Python runtime, independently of agent locks.
func ReadPythonLock(r io.Reader, arch string) (map[string]Spec, error) {
	return readLock(r, arch, []string{"python3"})
}

// BuildPython downloads a verified install_only runtime and preserves its full tree.
// As with Build, a failed build leaves incomplete output; use a fresh directory.
func BuildPython(ctx context.Context, client *http.Client, tools map[string]Spec, arch, destination string, output io.Writer) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported architecture %q", arch)
	}
	if err := validateSpecs(tools, []string{"python3"}); err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	spec := tools["python3"]
	fmt.Fprintln(output, "Installing python3", spec.Version)
	if err := install(ctx, client, root, "python3", spec); err != nil {
		return fmt.Errorf("install python3: %w", err)
	}
	return WriteManifest(root, arch, tools)
}
