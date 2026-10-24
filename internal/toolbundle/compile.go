package toolbundle

import (
	"bundledock/internal/toolconfig"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var zigTargets = map[string]string{"amd64": "x86_64-linux-musl", "arm64": "aarch64-linux-musl"}

// CompileArgs builds the compiler command line of a recipe: fixed flags and the
// recipe's allow-listed values only, in recipe order, with no shell involved. It
// repeats the catalog vocabulary check so no value outside it can reach the line.
func CompileArgs(p toolconfig.Package, arch, zig, srcDir, out string) ([]string, error) {
	if p.Build == nil {
		return nil, fmt.Errorf("no build block")
	}
	b := p.Build
	if err := b.CheckVocabulary("build"); err != nil {
		return nil, err
	}
	target, ok := zigTargets[arch]
	if !ok {
		return nil, fmt.Errorf("unsupported architecture %q", arch)
	}
	static := "-static"
	if b.OutputForm == "static-pie" {
		static = "-static-pie"
	}
	args := []string{zig, "cc", "-target", target, static}
	args = append(args, b.Cflags...)
	args = append(args, "-ffile-prefix-map="+srcDir+"=.")
	for _, d := range b.Defines {
		args = append(args, "-D"+d)
	}
	args = append(args, b.Sources...)
	for _, l := range b.Libs {
		args = append(args, "-l"+l)
	}
	return append(args, "-o", out), nil
}

// Compile runs the pinned compiler fetched by FetchSourceBuild (work/toolchain and
// work/src) on the recipe and leaves the output below outDir. The process gets a
// scrubbed environment and runs in the source directory; keeping the network away
// is the job of the image build stage (RUN --network=none), not of this function.
// It does not judge the output: the bundle build runs the ELF check.
func Compile(ctx context.Context, c *toolconfig.Config, tool, arch, work, outDir string, output io.Writer) error {
	p, ok := c.Catalog.Packages[tool]
	if !ok || p.Provider != "source-build" || p.Build == nil {
		return fmt.Errorf("%s is not a source-build tool", tool)
	}
	srcDir := filepath.Join(work, "src")
	out := filepath.Join(outDir, p.Build.Output)
	args, err := CompileArgs(p, arch, filepath.Join(work, "toolchain", "zig"), srcDir, out)
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Lstat(out); err == nil {
		return fmt.Errorf("%s: output %s already exists", tool, out)
	}
	cache := filepath.Join(work, "cache")
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = srcDir
	cmd.Env = []string{
		"SOURCE_DATE_EPOCH=1",
		"ZIG_GLOBAL_CACHE_DIR=" + filepath.Join(cache, "global"),
		"ZIG_LOCAL_CACHE_DIR=" + filepath.Join(cache, "local"),
	}
	cmd.Stdout, cmd.Stderr = output, output
	fmt.Fprintln(output, "Compiling", tool, "for", arch)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: compiler failed: %w", tool, err)
	}
	info, err := os.Lstat(out)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("%s: the compiler produced no regular output file %s", tool, p.Build.Output)
	}
	return nil
}
