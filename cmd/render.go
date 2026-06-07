package cmd

import (
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/render"
)

func newRenderCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "render [source...]",
		Short: "Render overlay layers and write the output files.",
		Long:  "Walk the source directories, render each group's active layers, and write the\nresult to the target directory. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + profilePrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := resolve(command, flags, args...)
			if err != nil {
				return err
			}
			return render.Run(render.Options{
				Settings:         r.Settings,
				ContinueOnError:  r.ContinueOnError,
				TOMLIndentTables: r.Effective.TOMLIndentTables,
				RenderRules:      r.Effective.RenderRules,
				Logger:           r.Logger,
			})
		},
	}
}
