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
		Long:  "Walk the source directories, render each group's active layers, and write the\nresult to the target directory. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + renderStateHelp + "\n" + profilePrecedenceHelp + "\n" + varsPrecedenceHelp,
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
	command.Flags().BoolVar(&noState, "no-state", false, "write rendered targets without reading or updating ownership state")
	return command
}
