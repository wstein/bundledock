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
	destination := flag.String("destination", "/opt/bundledock/toolbox", "bundle directory")
	flag.Parse()
	if err := run(*arch, *directory, *destination); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(arch, directory, destination string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported architecture %q", arch)
	}
	c, err := toolconfig.Load(directory)
	if err != nil {
		return err
	}
	tools := c.Lock.Platforms[arch]
	return toolbundle.Build(context.Background(), nil, tools, c.Catalog.Packages, os.DirFS(directory), arch, destination, os.Stdout)
}
