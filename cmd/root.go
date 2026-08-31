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
		Use:   "overlay",
		Short: "Merge layered JSON/TOML/YAML configuration files by profile.",
		Long: `Merge layered JSON, TOML, and YAML configuration files by profile, and copy
other profile-specific files through as whole files.

overlay walks one or more source directories for files named
<stem>.olay.<profile> or <stem>.olay.<profile>.<ext>, groups them by target
path, orders each group's active layers base, then the selected profiles in
list order, then local, and renders one file per group under the target
directory. 'overlay plan' prints the files a run would produce, 'overlay
diff' compares the rendered output with the existing target files, 'overlay
render' writes them, and 'overlay orphans' lists targets an earlier render
wrote that the current plan no longer produces.

Usage forms:
  overlay plan|diff|render|orphans [source...]
  overlay config [--validate PATH]
  overlay docs

` + configPrecedenceHelp + `

Only 'overlay render' writes: the rendered files under the target directory
and the ownership registry .overlay.state.json beside the loaded config
file. Every other command only reads. No command runs an external program
or accesses the network.

` + streamContractHelp + `

Run 'overlay <command> --help' for each command's contract, 'overlay config
--help' for the configuration report, 'overlay help exit-codes' for exit
statuses, and 'overlay docs' for the .overlay.toml schema reference.`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.bindPersistentFlags(root)

	root.AddCommand(
		newRenderCmd(flags),
		newDiffCmd(flags),
		newOrphansCmd(flags),
		newPlanCmd(flags),
		newConfigCmd(flags),
		newDocsCmd(),
		exitCodesTopic(),
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
