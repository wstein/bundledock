// python-bundle builds the separate checksum-pinned Python runtime image.
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
	lock := flag.String("lock", "/build/python.lock.json", "artifact lock")
	destination := flag.String("destination", "/opt/anchorage-python", "bundle directory")
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
	tools, err := toolbundle.ReadPythonLock(f, arch)
	if err != nil {
		return err
	}
	return toolbundle.BuildPython(context.Background(), nil, tools, arch, destination, os.Stdout)
}
