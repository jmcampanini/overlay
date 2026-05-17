package cli

import (
	"github.com/spf13/cobra"
)

// globals holds the flag values across all subcommands.
var globals GlobalFlags

// Execute parses the command line and runs the requested subcommand.
// It is called from cmd/overlay/main.go.
func Execute() error {
	root := newRootCmd()
	return root.Execute()
}

func newRootCmd() *cobra.Command {
	globals = GlobalFlags{}
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
