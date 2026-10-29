package cli

import "github.com/spf13/cobra"

func completionFishCommand() *cobra.Command {
	var noDescriptions bool
	cmd := &cobra.Command{
		Use:     "fish",
		Short:   "Generate a Fish completion script",
		Long:    "Generate a completion script for Fish.\n\n" + completionHelp,
		Example: "  overlay completion fish | source",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), !noDescriptions)
		},
	}
	cmd.Flags().BoolVar(&noDescriptions, "no-descriptions", false, "omit completion descriptions")
	return cmd
}
