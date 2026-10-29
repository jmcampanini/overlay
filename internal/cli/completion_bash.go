package cli

import "github.com/spf13/cobra"

func completionBashCommand() *cobra.Command {
	var noDescriptions bool
	cmd := &cobra.Command{
		Use:   "bash",
		Short: "Generate a Bash completion script",
		Long: `Generate a completion script for Bash. Loading it requires the
bash-completion package in your shell environment.

` + completionHelp,
		Example: "  source <(overlay completion bash)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), !noDescriptions)
		},
	}
	cmd.Flags().BoolVar(&noDescriptions, "no-descriptions", false, "omit completion descriptions")
	return cmd
}
