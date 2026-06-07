// Package cmd wires the cobra commands for the overlay binary.
package cmd

import (
	"github.com/jmcampanini/go-config-loader/pflagloader"
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
)

type globalFlags struct {
	config  string
	quiet   bool
	verbose bool
}

// Execute parses the command line and runs the requested subcommand.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	globalFlags := &globalFlags{}
	root := &cobra.Command{
		Use:           "overlay",
		Short:         "Merge layered JSON/TOML configuration files by profile.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	f := root.PersistentFlags()
	f.StringVar(&globalFlags.config, "config", "", "path to .overlay.toml (default: ./.overlay.toml)")
	f.BoolVarP(&globalFlags.quiet, "quiet", "q", false, "suppress INFO logs (show WARN and above)")
	f.BoolVarP(&globalFlags.verbose, "verbose", "v", false, "enable DEBUG logging")
	if err := pflagloader.Register[config.Config](f); err != nil {
		panic(err)
	}

	root.AddCommand(newRenderCmd(globalFlags))
	root.AddCommand(newDiffCmd(globalFlags))
	root.AddCommand(newPlanCmd(globalFlags))
	root.AddCommand(newConfigCmd(globalFlags))
	root.AddCommand(newDocsCmd())
	return root
}

func changed(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name) || cmd.PersistentFlags().Changed(name)
}
