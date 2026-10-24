package main

import (
	"bundledock/internal/toolbundle"
	"bundledock/internal/toolconfig"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

// sourceBuildOptions are the arguments of the two source-build subcommands:
//
//	tools-bundle fetch-sources -tool T -arch A [-directory D] -work W
//	tools-bundle compile       -tool T -arch A [-directory D] -work W -output O
//
// fetch-sources runs in the network stage, compile in the stage without network.
type sourceBuildOptions struct {
	tool, arch, directory, work, output string
}

func parseSourceBuild(command string, args []string, stderr io.Writer) (sourceBuildOptions, error) {
	var o sourceBuildOptions
	set := flag.NewFlagSet("tools-bundle "+command, flag.ContinueOnError)
	set.SetOutput(stderr)
	set.StringVar(&o.tool, "tool", "", "source-build tool name")
	set.StringVar(&o.arch, "arch", "", "target architecture (amd64 or arm64)")
	set.StringVar(&o.directory, "directory", "/build", "directory containing packages.txt, packages.catalog.json and packages.lock.json")
	set.StringVar(&o.work, "work", "", "work directory: toolchain/ and src/")
	if command == "compile" {
		set.StringVar(&o.output, "output", "", "directory receiving the compiled output")
	}
	if err := set.Parse(args); err != nil {
		return o, err
	}
	if set.NArg() != 0 {
		return o, fmt.Errorf("%s: unexpected arguments %q", command, set.Args())
	}
	if o.arch != "amd64" && o.arch != "arm64" {
		return o, fmt.Errorf("unsupported architecture %q", o.arch)
	}
	if o.tool == "" || o.work == "" {
		return o, fmt.Errorf("%s: -tool and -work are required", command)
	}
	if command == "compile" && o.output == "" {
		return o, fmt.Errorf("compile: -output is required")
	}
	return o, nil
}

func runSourceBuild(command string, args []string) error {
	o, err := parseSourceBuild(command, args, os.Stderr)
	if err != nil {
		return err
	}
	c, err := toolconfig.Load(o.directory)
	if err != nil {
		return err
	}
	if command == "fetch-sources" {
		return toolbundle.FetchSourceBuild(context.Background(), nil, c, o.tool, o.arch, o.work, os.Stdout)
	}
	return toolbundle.Compile(context.Background(), c, o.tool, o.arch, o.work, o.output, os.Stdout)
}
