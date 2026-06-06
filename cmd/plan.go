package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/cli"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/logging"
	"github.com/jmcampanini/overlay/internal/plan"
)

func newPlanCmd(globalFlags *cli.GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "plan [source...]",
		Short: "Show what files would be generated without writing anything.",
		Long:  "Print an aligned table of target paths, render modes, and active layers\nfor the current profile selection. Does not write any files. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + profilePrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := cli.Resolve(command, globalFlags, args...)
			if err != nil {
				return err
			}
			result, err := discover.WalkDetailed(r.Settings)
			if err != nil {
				return err
			}
			logging.WarnMissingSources(r.Logger, result.MissingSources)
			for _, stem := range result.Inactive {
				r.Logger.Infof("skipping %s (no active layers)", stem)
			}
			return plan.RenderWithOptions(
				os.Stdout,
				result.Active,
				r.Settings.Profiles,
				r.SourceLabels,
				r.Settings.TargetDir,
				plan.Options{RenderRules: r.Effective.RenderRules},
			)
		},
	}
}
