package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/diff"
)

func newDiffCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "diff [source...]",
		Short: "Show a unified diff between rendered output and existing target files.",
		Long:  "Render each overlay group in memory and print a unified diff against the\nexisting target files. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + diffOutputHelp + "\n" + profilePrecedenceHelp + "\n" + varsPrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := resolve(command, flags, args...)
			if err != nil {
				if _, writeErr := fmt.Fprintln(command.ErrOrStderr(), "overlay:", err); writeErr != nil {
					return ExitCode(2)
				}
				return ExitCode(2)
			}
			hasDiff, err := diff.Run(diff.Options{
				Settings:          r.Settings,
				ContinueOnError:   r.ContinueOnError,
				TOMLIndentTables:  r.Effective.TOMLIndentTables,
				RenderRules:       r.Effective.RenderRules,
				Substituter:       r.Substituter,
				SubstituteExclude: r.SubstituteExclude,
				Logger:            r.Logger,
				Out:               command.OutOrStdout(),
			})
			if err != nil {
				r.Logger.Error(err)
				return ExitCode(2)
			}
			if hasDiff {
				return ExitCode(1)
			}
			return nil
		},
		SilenceErrors: true,
	}
}
