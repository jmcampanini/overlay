// Package diff computes in-memory unified diffs between rendered overlay
// outputs and their existing target files.
package diff

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"charm.land/log/v2"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/logging"
	"github.com/jmcampanini/overlay/internal/render"
)

// Options carries everything Run needs.
type Options struct {
	Settings         discover.Settings
	ContinueOnError  bool
	TOMLIndentTables bool
	RenderRules      []config.RenderRule
	Logger           *log.Logger
	Out              io.Writer // diff output goes here; defaults to os.Stdout
}

// Run discovers groups, renders each in memory, and writes a unified
// diff to Out for every file that differs from the existing target.
// Returns (anyDiffer, err). anyDiffer is true if at least one group
// produced a non-empty diff. err is non-nil on setup failure or when
// a render/compare failed (and ContinueOnError is false).
func Run(opts Options) (bool, error) {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if err := config.ValidateRenderRules(opts.RenderRules); err != nil {
		return false, err
	}

	result, err := discover.WalkDetailed(opts.Settings)
	if err != nil {
		return false, fmt.Errorf("discover: %w", err)
	}
	logging.WarnMissingSources(opts.Logger, result.MissingSources)
	for _, stem := range result.Inactive {
		opts.Logger.Infof("skipping %s (no active layers)", stem)
	}
	groups := result.Active
	if len(groups) == 0 {
		opts.Logger.Debugf("no overlay files found in %s", sourceSummary(opts.Settings))
		return false, nil
	}

	mergeOptions := render.MergeOptions{
		TOMLIndentTables: opts.TOMLIndentTables,
		RenderRules:      opts.RenderRules,
		TargetDir:        opts.Settings.TargetDir,
	}
	var anyDiffer bool
	var failed int
	for _, g := range groups {
		rendered, err := render.MergeGroupWithOptions(g, mergeOptions)
		if err != nil {
			if opts.ContinueOnError {
				opts.Logger.Errorf("render %s: %v", g.TargetPath, err)
				failed++
				continue
			}
			return anyDiffer, fmt.Errorf("render %s: %w", g.TargetPath, err)
		}
		existing, err := readTarget(g.TargetPath)
		if err != nil {
			if opts.ContinueOnError {
				opts.Logger.Errorf("read %s: %v", g.TargetPath, err)
				failed++
				continue
			}
			return anyDiffer, fmt.Errorf("read %s: %w", g.TargetPath, err)
		}
		out := Unified(existing, rendered, "a/"+g.TargetPath, "b/"+g.TargetPath)
		if out != "" {
			anyDiffer = true
			if _, err := fmt.Fprint(opts.Out, out); err != nil {
				return anyDiffer, fmt.Errorf("write diff: %w", err)
			}
		}
	}
	if failed > 0 {
		return anyDiffer, fmt.Errorf("%d files failed during diff", failed)
	}
	return anyDiffer, nil
}

func sourceSummary(settings discover.Settings) string {
	return strings.Join(settings.SourceDirs, ", ")
}

// readTarget returns the target file's bytes. A missing target file is
// treated as empty content (the diff will show all lines as additions).
func readTarget(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}
