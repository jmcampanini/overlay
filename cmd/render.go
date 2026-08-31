package cmd

import (
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/render"
)

func newRenderCmd(flags *globalFlags) *cobra.Command {
	var noState bool
	command := &cobra.Command{
		Use:   "render [source...]",
		Short: "Render overlay layers and write the output files.",
		Long: `Walk the source directories, render each group's active layers, and write the
result to the target directory. Positional sources select package roots for
this run.

Every group is composed in memory first. When any group fails to compose
(an unparsable layer, missing variables), nothing is written and the error
names every failing target; with --continue or continue_on_error, the clean
targets are written, each failure is logged, and the run still exits 1.
Each target is created or overwritten in place with mode 0644 after its
parent directories are created with mode 0755, and an existing symlink at
the target path is written through to the file it points to. Each written
target is logged on stderr with its layer list, followed by 'overlayed N
files'; nothing goes to stdout. Exit status is 0 when every target was
written and 1 otherwise. Reads the config file, the source directories, and
the state file; writes only the targets and the state file.

` + sourceSelectionHelp + `

` + fileConventionHelp + `

` + renderStateHelp + `

` + streamContractHelp + `

` + profilePrecedenceHelp + `

` + mergeSemanticsHelp + `

` + varsPrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := resolve(command, flags, args...)
			if err != nil {
				return err
			}
			return render.Run(render.Options{
				Settings:          r.Settings,
				ContinueOnError:   r.ContinueOnError,
				TOMLIndentTables:  r.Effective.TOMLIndentTables,
				RenderRules:       r.Effective.RenderRules,
				Substituter:       r.Substituter,
				SubstituteExclude: r.SubstituteExclude,
				StatePath:         r.StatePath,
				NoState:           noState,
				Logger:            r.Logger,
			})
		},
	}
	command.Flags().BoolVar(&noState, "no-state", false, "write targets without reading or updating ownership state")
	return command
}
