// tools-init runs inside the OCI bundle image to initialize its mounted volume.
package main

import (
	"anchorage/internal/toolvolume"
	"fmt"
	"os"
)

func main() {
	if err := toolvolume.Initialize("/opt/anchorage", "/bundle", os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
