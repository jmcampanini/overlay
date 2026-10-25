// Package cli defines overlay's CLI commands and output contracts.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Version is injected at build time by the mise build task or package manager.
var Version = "n/a"

// Execute runs the command tree and returns the process exit status.
func Execute(args []string, out, errOut io.Writer) int {
	root := newRootCommand(out, errOut)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintf(errOut, "overlay: error: %v\n", err)
		return 1
	}
	return 0
}

// newRootCommand builds the overlay command tree.
func newRootCommand(out, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "overlay",
		Short: "Render layered dotfiles from source repositories",
		Long: `Render layered dotfiles from git source repositories into a target
directory.

overlay reads one configuration file that lists the layers to apply and the
git repositories that provide files. 'overlay bootstrap' writes that file
and clones the sources; 'overlay config' prints the configuration in
effect; 'overlay render' copies the files the layers select into the
target directory, or with --dry-run only prints what it would write.

` + configFileHelp + `

Only 'overlay bootstrap' runs an external program: it runs git from PATH to
clone sources and to read the origin of an existing checkout, and never
lets git prompt for credentials. No command prompts and no command reads
the network on its own.

Run 'overlay config --help' for configuration precedence and the file
schema, 'overlay render --help' for the source manifest and the rendering
rules, and 'overlay help exit-codes' for exit-status meanings.`,
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
		renderCommand(),
	)

	return cmd
}
