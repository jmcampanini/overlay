package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/plan"
)

func newPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "plan [source...]",
		Short: "Show what files would be generated without writing anything.",
		Long:  "Print an aligned table of target paths, formats, and active layers\nfor the current profile selection. Does not write any files. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + profilePrecedenceHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := Resolve(cmd, &globals, args...)
			if err != nil {
				return err
			}
			active, inactive, err := discover.Walk(r.Settings)
			if err != nil {
				return err
			}
			for _, stem := range inactive {
				r.Logger.Infof("skipping %s (no active layers)", stem)
			}
			return plan.Render(os.Stdout, active, r.Settings.Profiles, r.SourceLabels, r.Settings.TargetDir)
		},
	}
}
