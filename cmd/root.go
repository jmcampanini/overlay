// Package cmd wires the cobra commands for the overlay binary.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/cli"
)

// Execute parses the command line and runs the requested subcommand.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	globalFlags := &cli.GlobalFlags{}
	root := &cobra.Command{
		Use:           "overlay",
		Short:         "Merge layered JSON/TOML configuration files by profile.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	globalFlags.Bind(root)
	root.AddCommand(newRenderCmd(globalFlags))
	root.AddCommand(newDiffCmd(globalFlags))
	root.AddCommand(newPlanCmd(globalFlags))
	root.AddCommand(newConfigCmd(globalFlags))
	root.AddCommand(newDocsCmd())
	return root
}
