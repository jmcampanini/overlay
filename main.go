// Package main runs overlay's layered dotfile commands.
package main

import (
	"fmt"
	"os"

	"github.com/jmcampanini/overlay/cmd"
)

func main() {
	root := cmd.NewRootCommand(os.Stdout, os.Stderr)
	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "overlay: error: %v\n", err)
		os.Exit(1)
	}
}
