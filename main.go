// Package main runs overlay's layered dotfile commands.
package main

import (
	"os"

	"github.com/jmcampanini/overlay/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
