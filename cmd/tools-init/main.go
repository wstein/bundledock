// tools-init runs inside the OCI bundle image to initialize its mounted volume.
package main

import (
	"anchorage/internal/toolvolume"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], "/bundle", os.Stdout); err != nil && err != flag.ErrHelp {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run keeps the existing agent-tools source unless an image selects its own tree.
func run(args []string, target string, output io.Writer) error {
	flags := flag.NewFlagSet("init-volume", flag.ContinueOnError)
	flags.SetOutput(output)
	source := flags.String("source", "/opt/anchorage", "bundle source directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected initializer arguments")
	}
	return toolvolume.Initialize(*source, target, output)
}
