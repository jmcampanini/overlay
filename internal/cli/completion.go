package cli

import "github.com/spf13/cobra"

const completionHelp = `The script goes to stdout and can be sourced or redirected by your shell.
--no-descriptions omits descriptions from completion candidates. No files
are read or changed, and configuration is not loaded. There are no JSON,
JSONL, or CSV formats.

` + contractHelp

func completionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long: `Generate an overlay completion script with one of the shell subcommands.
Each subcommand's --help shows how to load its script. A bare invocation
prints this help on stdout. No configuration or source files are read.
The output is human-readable help or a shell script, with no other formats.

` + contractHelp,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		completionBashCommand(),
		completionFishCommand(),
		completionPowerShellCommand(),
		completionZshCommand(),
	)
	return cmd
}
