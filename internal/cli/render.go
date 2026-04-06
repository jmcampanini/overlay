package cli

import (
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/render"
)

func newRenderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "render",
		Short: "Merge overlay layers and write the output files.",
		Long:  "Walk the source directory, merge each group's active layers, and write the\nresult to the target directory.\n" + profilePrecedenceHelp,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := Resolve(cmd, &globals)
			if err != nil {
				return err
			}
			return render.Run(render.Options{
				Settings:        r.Settings,
				ContinueOnError: r.ContinueOnError,
				Logger:          r.Logger,
			})
		},
	}
}
