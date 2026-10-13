// tools-bundle is the image build's native artifact installer.
package main

import (
	"bundledock/internal/toolbundle"
	"bundledock/internal/toolconfig"
	"context"
	"flag"
	"fmt"
	"os"
)

func main() {
	arch := flag.String("arch", "", "target architecture (amd64 or arm64)")
	directory := flag.String("directory", "/build", "directory containing packages.txt, packages.catalog.json and packages.lock.json")
	destination := flag.String("destination", "/opt/bdock/toolbox", "bundle directory")
	gitTrimStage := flag.String("git-trim-stage", "", "sealed output of the fixed git-trim Docker recipe")
	gitTrimSource := flag.String("git-trim-source", "", "source directory for the fixed git-trim recipe")
	gitTrimMode := flag.String("git-trim-mode", "", "fixed git-trim recipe operation: prepare or seal")
	flag.Parse()
	if err := runWithGitTrim(*arch, *directory, *destination, *gitTrimSource, *gitTrimStage, *gitTrimMode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(arch, directory, destination string) error {
	return runWithGitTrim(arch, directory, destination, "", "", "")
}
func runWithGitTrim(arch, directory, destination, source, stage, mode string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported architecture %q", arch)
	}
	c, err := toolconfig.Load(directory)
	if err != nil {
		return err
	}
	tools := c.Lock.Platforms[arch]
	switch mode {
	case "prepare":
		return toolbundle.PrepareGitTrim(context.Background(), tools["git-trim"], source)
	case "seal":
		return toolbundle.SealGitTrim(tools["git-trim"], arch, source, stage)
	case "":
	default:
		return fmt.Errorf("unsupported git-trim operation %q", mode)
	}
	return toolbundle.BuildWithGitTrimStage(context.Background(), nil, tools, c.Catalog.Packages, os.DirFS(directory), arch, destination, stage, os.Stdout)
}
