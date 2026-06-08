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
	flags := &globalFlags{}
	root := &cobra.Command{
		Use:           "overlay",
		Short:         "Merge layered JSON/TOML/YAML configuration files by profile.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.bindPersistentFlags(root)

	root.AddCommand(
		newRenderCmd(flags),
		newDiffCmd(flags),
		newPlanCmd(flags),
		newConfigCmd(flags),
		newDocsCmd(),
	)
	return root
}

func (g *globalFlags) bindPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.StringVar(&g.config, "config", "", "path to .overlay.toml (default: ./.overlay.toml)")
	f.BoolVarP(&g.quiet, "quiet", "q", false, "suppress INFO logs (show WARN and above)")
	f.BoolVarP(&g.verbose, "verbose", "v", false, "enable DEBUG logging")
	if err := pflagloader.Register[config.Config](f); err != nil {
		panic(err)
	}
}

func changed(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name) || cmd.PersistentFlags().Changed(name)
}
