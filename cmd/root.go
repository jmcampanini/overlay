// Package cmd defines overlay's CLI commands and output contracts.
package cmd

import (
	"io"

	"github.com/spf13/cobra"
)

// NewRootCommand builds the overlay command tree.
func NewRootCommand(out, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "overlay",
		Short: "Render layered dotfiles from source repositories",
		Long: `Render layered dotfiles from git source repositories into a target
directory.

overlay reads one configuration file that lists the layers to apply and the
git repositories that provide files. 'overlay bootstrap' writes that file
and clones the sources; 'overlay config' prints the configuration in
effect. Rendering commands arrive in later releases.

` + configFileHelp + `

Only 'overlay bootstrap' runs an external program: it runs git from PATH to
clone sources and to read the origin of an existing checkout, and never
lets git prompt for credentials. No command prompts and no command reads
the network on its own.

Run 'overlay config --help' for configuration precedence and the file
schema, and 'overlay help exit-codes' for exit-status meanings.`,
		Version:            Version,
		DisableSuggestions: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
	}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	// Register --help and --version before Find strips flags from the
	// arguments. Cobra otherwise adds them during execute, after Find has
	// already treated an unregistered --help or --version as taking the next
	// argument as its value.
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()

	cmd.AddCommand(
		bootstrapCommand(),
		configCommand(),
		exitCodesTopic(),
	)

	return cmd
}
