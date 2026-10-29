package cli

import "github.com/spf13/cobra"

func completionPowerShellCommand() *cobra.Command {
	var noDescriptions bool
	cmd := &cobra.Command{
		Use:     "powershell",
		Short:   "Generate a PowerShell completion script",
		Long:    "Generate a completion script for PowerShell.\n\n" + completionHelp,
		Example: "  overlay completion powershell | Out-String | Invoke-Expression",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if noDescriptions {
				return cmd.Root().GenPowerShellCompletion(cmd.OutOrStdout())
			}
			return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&noDescriptions, "no-descriptions", false, "omit completion descriptions")
	return cmd
}
