// Package dockercli provides the host's narrow Docker process adapter.
package dockercli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

type Runner func(context.Context, io.Writer, io.Writer, ...string) error

// Run invokes Docker without shell interpolation and cancels it with the context.
func Run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

var imageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Volume resolves a mutable image reference once and initializes using that ID.
func Volume(ctx context.Context, run Runner, image string, stdout, stderr io.Writer) error {
	var resolved bytes.Buffer
	if err := run(ctx, &resolved, stderr, "image", "inspect", "--format", "{{.Id}}", "--", image); err != nil {
		return fmt.Errorf("inspect tools image: %w", err)
	}
	id := strings.TrimSpace(resolved.String())
	if !imageID.MatchString(id) {
		return fmt.Errorf("Docker returned invalid image ID %q", id)
	}
	volume := "bundledock-toolbox-" + strings.TrimPrefix(id, "sha256:")
	if err := run(ctx, io.Discard, stderr, "volume", "create", volume); err != nil {
		return fmt.Errorf("create tools volume: %w", err)
	}
	if err := run(ctx, stdout, stderr, "run", "--rm", "--network", "none", "--mount", "type=volume,src="+volume+",dst=/bundle,volume-nocopy", id); err != nil {
		return fmt.Errorf("initialize tools volume: %w", err)
	}
	_, err := fmt.Fprintln(stdout, "Volume:", volume)
	return err
}
