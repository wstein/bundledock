// tools-bundle is the image build's native artifact installer.
package main

import (
	"bundledock/internal/toolbundle"
	"bundledock/internal/toolconfig"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "fetch-sources" || os.Args[1] == "compile") {
		if err := runSourceBuild(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "normalize-mtimes" {
		if err := runNormalize(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	arch := flag.String("arch", "", "target architecture (amd64 or arm64)")
	directory := flag.String("directory", "/build", "directory containing packages.txt, packages.catalog.json and packages.lock.json")
	destination := flag.String("destination", "/opt/bdock/toolbox", "bundle directory")
	shellLayer := flag.String("shell-layer", "", "toolbox only: source directory of the shared shell layer (internal/shelllayer)")
	buildOutputs := flag.String("build-outputs", "", "directory with the compile-stage output of each source-build tool (<dir>/<tool>/<output>)")
	flag.Parse()
	if err := run(*arch, *directory, *destination, *shellLayer, *buildOutputs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(arch, directory, destination, shellLayer, buildOutputs string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported architecture %q", arch)
	}
	c, err := toolconfig.Load(directory)
	if err != nil {
		return err
	}
	tools := c.Lock.Platforms[arch]
	var opts []toolbundle.BuildOption
	if shellLayer != "" {
		opts = append(opts, toolbundle.WithShellLayer(os.DirFS(shellLayer)))
	}
	if buildOutputs != "" {
		opts = append(opts, toolbundle.WithBuildOutputs(buildOutputs))
	}
	return toolbundle.Build(context.Background(), nil, tools, c.Catalog.Packages, os.DirFS(directory), arch, destination, os.Stdout, opts...)
}

// runNormalize fixes every modification time below -root so that image layers
// do not depend on the build time. The epoch defaults to SOURCE_DATE_EPOCH.
func runNormalize(args []string) error {
	fs := flag.NewFlagSet("normalize-mtimes", flag.ContinueOnError)
	root := fs.String("root", "", "directory whose entries get the fixed time")
	epoch := fs.String("epoch", os.Getenv("SOURCE_DATE_EPOCH"), "Unix time in seconds (default: SOURCE_DATE_EPOCH)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" || *epoch == "" {
		return fmt.Errorf("normalize-mtimes needs -root and an epoch (-epoch or SOURCE_DATE_EPOCH)")
	}
	n, err := strconv.ParseInt(*epoch, 10, 64)
	if err != nil {
		return fmt.Errorf("epoch %q: %w", *epoch, err)
	}
	return toolbundle.NormalizeMtimes(*root, n)
}
