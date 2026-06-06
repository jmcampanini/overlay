// Package cmd wires the cobra commands for the overlay binary.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/cli"
)

// globals holds the flag values across all subcommands.
var globals cli.GlobalFlags

// Execute parses the command line and runs the requested subcommand.
func Execute() error {
	root := newRootCmd()
	return root.Execute()
}

func newRootCmd() *cobra.Command {
	globals = cli.GlobalFlags{}
	root := &cobra.Command{
		Use:           "overlay",
		Short:         "Merge layered JSON/TOML configuration files by profile.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	globals.Bind(root)
	root.AddCommand(newRenderCmd())
	root.AddCommand(newDiffCmd())
	root.AddCommand(newPlanCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newDocsCmd())
	return root
}
