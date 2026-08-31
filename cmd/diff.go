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
		Long: `Render each overlay group in memory and print a unified diff against the
existing target files. Positional sources select package roots for this run.

Every group is composed in memory and compared with the file at its target
path; a target that does not exist yet diffs as all additions. Nothing is
written. Reads the config file, the source directories, and the target
files. A group that fails to compose (an unparsable layer, missing
variables) stops the run before any diff is printed, and a target that
cannot be read stops it where it is, both with exit 2; with --continue or
continue_on_error each failure is logged, every clean diff is printed, and
the exit status is still 2. Resolution errors print 'overlay: <message>' on
stderr; later errors print as ERRO lines.

` + sourceSelectionHelp + `

` + fileConventionHelp + `

` + diffOutputHelp + `

` + streamContractHelp + `

` + profilePrecedenceHelp + `

` + mergeSemanticsHelp + `

` + varsPrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := resolve(command, flags, args...)
			if err != nil {
				_, _ = fmt.Fprintln(command.ErrOrStderr(), "overlay:", err)
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
