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
	directory := flag.String("directory", "/build", "directory containing tools.txt, tools.packages.json and tools.lock.json")
	sqliteSource := flag.String("sqlite-source", "", "download verified SQLite build sources to this directory instead of building the bundle")
	bashSource := flag.String("bash-source", "", "download verified GNU Bash base and ordered patches to this build-only directory")
	destination := flag.String("destination", "/opt/workharbor", "bundle directory")
	flag.Parse()
	if err := run(*arch, *directory, *destination, *sqliteSource, *bashSource); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(arch, directory, destination, sqliteSource, bashSource string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported architecture %q", arch)
	}
	c, err := toolconfig.Load(directory)
	if err != nil {
		return err
	}
	if sqliteSource != "" && bashSource != "" {
		return fmt.Errorf("select one source preparation recipe")
	}
	tools := c.Lock.Platforms[arch]
	if bashSource != "" {
		return toolbundle.PrepareBashSource(context.Background(), nil, tools["bash"], bashSource)
	}
	if sqliteSource != "" {
		return toolbundle.PrepareSQLiteSource(context.Background(), nil, tools["sqlite3"], c.Catalog.Packages["sqlite3"], sqliteSource)
	}
	return toolbundle.Build(context.Background(), nil, tools, c.Catalog.Packages, os.DirFS(directory), arch, destination, os.Stdout)
}
