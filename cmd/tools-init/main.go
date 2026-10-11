// tools-init runs inside the OCI bundle image to initialize its mounted volume.
package main

import (
	"bundledock/internal/toolvolume"
	"flag"
	"fmt"
	"os"
)

func main() {
	source := flag.String("source", "/opt/bundledock/toolbox", "bundle directory in the image")
	target := flag.String("target", "/bundle", "mounted tool volume directory")
	profile := flag.String("profile", "", "explicit optional bundle profile (code-server-standalone)")
	flag.Parse()
	if err := toolvolume.InitializeWithProfile(*source, *target, *profile, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
