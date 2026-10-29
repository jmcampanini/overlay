package cli

import "github.com/spf13/cobra"

func completionZshCommand() *cobra.Command {
	var noDescriptions bool
	cmd := &cobra.Command{
		Use:   "zsh",
		Short: "Generate a Zsh completion script",
		Long: `Generate a completion script for Zsh. Enable Zsh completion before loading
the script.

` + completionHelp,
		Example: "  autoload -U compinit; compinit\n  source <(overlay completion zsh)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if noDescriptions {
				return cmd.Root().GenZshCompletionNoDesc(cmd.OutOrStdout())
			}
			return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&noDescriptions, "no-descriptions", false, "omit completion descriptions")
	return cmd
}
