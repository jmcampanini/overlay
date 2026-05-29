package cli

import (
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/render"
)

func newRenderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "render [source...]",
		Short: "Render overlay layers and write the output files.",
		Long:  "Walk the source directories, render each group's active layers, and write the\nresult to the target directory. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + profilePrecedenceHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := Resolve(cmd, &globals, args...)
			if err != nil {
				return err
			}
			return render.Run(render.Options{
				Settings:         r.Settings,
				ContinueOnError:  r.ContinueOnError,
				TOMLIndentTables: r.RawConfig.TOMLIndentTables,
				RenderRules:      r.RawConfig.RenderRules,
				Logger:           r.Logger,
			})
		},
	}
}
