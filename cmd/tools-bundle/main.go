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
	destination := flag.String("destination", "/opt/anchorage", "bundle directory")
	flag.Parse()
	if err := run(*arch, *lock, *destination); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(arch, lock, destination string) error {
	f, err := os.Open(lock)
	if err != nil {
		return err
	}
	defer f.Close()
	tools, err := toolbundle.ReadLock(f, arch)
	if err != nil {
		return err
	}
	return toolbundle.Build(context.Background(), nil, tools, arch, destination, os.Stdout)
}
