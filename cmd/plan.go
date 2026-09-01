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
		Long: `Print an aligned table of target paths, render modes, and active layers
for the current profile selection. Does not write any files. Positional
sources select package roots for this run.

The table goes to stdout between an 'Active profiles:' and 'Sources:'
header and an 'N files will be generated' summary, with paths under the
home directory shortened to ~. Columns are TARGET, MODE (merge, copy, or
append), LAYERS (the active layers in order, with '(winner: NAME)' for
copy), and, when substitute is set, VARS listing each
substituting target's variables with unset ones marked '(missing!)'. The
header row is bold and the table keeps its ANSI escapes even when stdout
is not a terminal or NO_COLOR is set. Substituting targets are composed in
memory so missing variables are found; other targets are not parsed, so
their parse errors surface only in render or diff. Reads the config file
and the source directories. Exits 1 when resolution or discovery fails or
when a substituting target has missing variables or cannot compose (the
error names every failing target); --continue does not change that.

` + sourceSelectionHelp + `

` + fileConventionHelp + `

` + streamContractHelp + `

` + profilePrecedenceHelp + `

` + mergeSemanticsHelp + `

` + varsPrecedenceHelp,
		Example: `  # .overlay.toml: profiles = ["base-tools"]
  #                env_profiles = ["DOTFILES_PROFILE", "HOST_PROFILE"]
  overlay plan                            # Active profiles: [base-tools]
  DOTFILES_PROFILE=work overlay plan      # [base-tools, work]
  DOTFILES_PROFILE=work overlay --profiles personal plan   # [personal, work]
  overlay --profile personal --profile client plan      # [personal, client]
  DOTFILES_PROFILE=work,laptop HOST_PROFILE=laptop overlay plan
                                          # [base-tools, work, laptop]
  overlay plan pi codex                   # only the pi and codex roots`,
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
