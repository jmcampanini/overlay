package cmd

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/orphans"
	"github.com/jmcampanini/overlay/internal/state"
)

func newOrphansCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "orphans [source...]",
		Short: "Show rendered targets that are no longer produced by the active plan.",
		Long:  "Compare the render state with the current active plan and print stale target\npaths. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + orphansOutputHelp + "\n" + profilePrecedenceHelp,
		RunE: func(command *cobra.Command, args []string) error {
			r, err := resolve(command, flags, args...)
			if err != nil {
				if _, writeErr := fmt.Fprintln(command.ErrOrStderr(), "overlay:", err); writeErr != nil {
					return ExitCode(2)
				}
				return ExitCode(2)
			}

			entries, err := state.Load(r.StatePath)
			if err != nil {
				if errors.Is(err, state.ErrNotExist) {
					r.Logger.Error("no state file yet; run `overlay render` to establish the baseline")
				} else {
					r.Logger.Error(err)
				}
				return ExitCode(2)
			}

			result, err := discover.WalkDetailed(r.Settings)
			if err != nil {
				r.Logger.Error(fmt.Errorf("discover: %w", err))
				return ExitCode(2)
			}
			for _, source := range result.MissingSources {
				r.Logger.Warnf("source %q not found, skipping", source)
			}
			for _, stem := range result.Inactive {
				r.Logger.Infof("skipping %s (no active layers)", stem)
			}

			planTargets, err := activeTargetSet(result.Active)
			if err != nil {
				r.Logger.Error(err)
				return ExitCode(2)
			}
			selectedSources, err := absolutePathSet(r.Settings.SourceDirs)
			if err != nil {
				r.Logger.Error(err)
				return ExitCode(2)
			}
			found := orphans.Detect(orphans.Options{
				Entries:         entries,
				PlanTargets:     planTargets,
				SelectedSources: selectedSources,
				Narrowed:        r.SourcesNarrowed,
			})
			for _, orphan := range found {
				if _, err := fmt.Fprintln(command.OutOrStdout(), orphan.Target); err != nil {
					r.Logger.Error(fmt.Errorf("write stdout: %w", err))
					return ExitCode(2)
				}
			}
			if len(found) > 0 {
				return ExitCode(1)
			}
			return nil
		},
		SilenceErrors: true,
	}
}

func activeTargetSet(groups []discover.Group) (map[string]struct{}, error) {
	paths := make([]string, len(groups))
	for i, group := range groups {
		paths[i] = group.TargetPath
	}
	return absolutePathSet(paths)
}

func absolutePathSet(paths []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve absolute path %q: %w", path, err)
		}
		set[absolute] = struct{}{}
	}
	return set, nil
}
