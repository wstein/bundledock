// tools-init runs inside the OCI bundle image to initialize its mounted volume.
package main

import (
	"bundledock/internal/toolvolume"
	"flag"
	"fmt"
	"os"
)

func main() {
	source := flag.String("source", "/opt/bundledock", "bundle directory in the image")
	target := flag.String("target", "/bundle", "mounted tool volume directory")
	flag.Parse()
	if err := toolvolume.Initialize(*source, *target, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
