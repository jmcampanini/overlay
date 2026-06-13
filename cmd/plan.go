package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/plan"
)

func newPlanCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "plan [source...]",
		Short: "Show what files would be generated without writing anything.",
		Long:  "Print an aligned table of target paths, render modes, and active layers\nfor the current profile selection. Does not write any files. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + profilePrecedenceHelp + "\n" + varsPrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := resolve(command, flags, args...)
			if err != nil {
				return err
			}
			result, err := discover.WalkDetailed(r.Settings)
			if err != nil {
				return err
			}
			for _, source := range result.MissingSources {
				r.Logger.Warnf("source %q not found, skipping", source)
			}
			for _, stem := range result.Inactive {
				r.Logger.Infof("skipping %s (no active layers)", stem)
			}
			return plan.RenderWithOptions(
				os.Stdout,
				result.Active,
				r.Settings.Profiles,
				r.SourceLabels,
				r.Settings.TargetDir,
				plan.Options{
					RenderRules:       r.Effective.RenderRules,
					TOMLIndentTables:  r.Effective.TOMLIndentTables,
					Substituter:       r.Substituter,
					SubstituteExclude: r.SubstituteExclude,
					Logger:            r.Logger,
				},
			)
		},
	}
}
