package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/diff"
)

// DiffExitCode wraps an exit code so main can detect it and exit without
// the generic "error" wrapping that would print an extra message.
type DiffExitCode int

// Error satisfies the error interface.
func (e DiffExitCode) Error() string { return fmt.Sprintf("diff exit code %d", int(e)) }

func newDiffCmd(globalFlags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "diff [source...]",
		Short: "Show a unified diff between rendered output and existing target files.",
		Long:  "Render each overlay group in memory and print a unified diff against the\nexisting target files. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + diffOutputHelp + "\n" + profilePrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := resolve(command, globalFlags, args...)
			if err != nil {
				fmt.Fprintln(os.Stderr, "overlay:", err)
				return DiffExitCode(2)
			}
			hasDiff, err := diff.Run(diff.Options{
				Settings:         r.Settings,
				ContinueOnError:  r.ContinueOnError,
				TOMLIndentTables: r.Effective.TOMLIndentTables,
				RenderRules:      r.Effective.RenderRules,
				Logger:           r.Logger,
				Out:              os.Stdout,
			})
			if err != nil {
				r.Logger.Error(err)
				return DiffExitCode(2)
			}
			if hasDiff {
				return DiffExitCode(1)
			}
			return nil
		},
		SilenceErrors: true,
	}
}
