// tools-init runs inside the OCI bundle image to initialize its mounted volume.
package main

import (
	"bundledock/internal/toolbundle"
	"bundledock/internal/toolvolume"
	"flag"
	"fmt"
	"os"
)

func main() {
	source := flag.String("source", "/opt/bundledock/toolbox", "bundle directory in the image")
	target := flag.String("target", "/bundle", "mounted tool volume directory")
	profile := flag.String("profile", "", "explicit optional bundle profile (code-server-standalone, node-runtime or bun-runtime)")
	list := flag.Bool("list-licenses", false, "verify and print the offline license index without initializing a volume")
	license := flag.String("license", "", "verify and print a complete licenses/package/path notice without initializing a volume")
	flag.Parse()
	licenseSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "license" {
			licenseSet = true
		}
	})
	if flag.NArg() != 0 || (*list && licenseSet) || (licenseSet && *license == "") {
		fmt.Fprintln(os.Stderr, "use either --list-licenses or --license licenses/package/path, without positional arguments")
		os.Exit(2)
	}
	if *list || licenseSet {
		root, err := os.OpenRoot(*source)
		if err == nil {
			err = toolbundle.ReadLicenses(root, *license, os.Stdout)
			root.Close()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := toolvolume.InitializeWithProfile(*source, *target, *profile, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
