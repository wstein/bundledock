// tools-bundle is the image build's standard-library-only artifact installer.
package main

import (
	"anchorage/internal/toolbundle"
	"context"
	"flag"
	"fmt"
	"os"
)

func main() {
	arch := flag.String("arch", "", "target architecture (amd64 or arm64)")
	lock := flag.String("lock", "/build/tools.lock.json", "artifact lock")
	sqliteSource := flag.String("sqlite-source", "", "download verified SQLite build sources to this directory instead of building the bundle")
	destination := flag.String("destination", "/opt/anchorage", "bundle directory")
	flag.Parse()
	if err := run(*arch, *lock, *destination, *sqliteSource); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(arch, lock, destination, sqliteSource string) error {
	f, err := os.Open(lock)
	if err != nil {
		return err
	}
	defer f.Close()
	tools, err := toolbundle.ReadLock(f, arch)
	if err != nil {
		return err
	}
	if sqliteSource != "" {
		return toolbundle.PrepareSQLiteSource(context.Background(), nil, tools["sqlite3"], sqliteSource)
	}
	return toolbundle.Build(context.Background(), nil, tools, arch, destination, os.Stdout)
}
