package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const helpTopicsGroupID = "help-topics"

func helpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command or topic]",
		Short: "Show help for a command or topic",
		Long: `Print help for a command or topic on stdout, or root help when none is
given. Nested command names follow their normal order. Unknown names and
extra operands are usage errors. No configuration or source files are read.
The output is human-readable text, with no machine-readable format.

` + contractHelp,
		Example: "  overlay help render\n  overlay help completion bash\n  overlay help exit-codes",
		Args: func(cmd *cobra.Command, args []string) error {
			_, err := findHelpCommand(cmd.Root(), args)
			return err
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			target, err := findHelpCommand(cmd.Root(), args)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var completions []cobra.Completion
			for _, child := range target.Commands() {
				if !child.Hidden && strings.HasPrefix(child.Name(), toComplete) {
					completions = append(completions, cobra.CompletionWithDesc(child.Name(), child.Short))
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := findHelpCommand(cmd.Root(), args)
			if err != nil {
				return err
			}
			target.InitDefaultHelpFlag()
			return target.Help()
		},
	}
}

func findHelpCommand(root *cobra.Command, args []string) (*cobra.Command, error) {
	target, remaining, err := root.Find(args)
	if err != nil || len(remaining) != 0 {
		return nil, fmt.Errorf("unknown help topic %q; run 'overlay help' for available commands and topics", strings.Join(args, " "))
	}
	return target, nil
}
