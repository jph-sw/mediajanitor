package main

import (
	"fmt"
	"os"

	"github.com/jph-sw/mediajanitor/internal/cli"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	root := cli.New(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
