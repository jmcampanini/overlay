// Package render orchestrates the full overlay pipeline: discover groups,
// merge their layers, serialize, and write the outputs to disk.
package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
	"github.com/jmcampanini/overlay/internal/merge"
)

// Options carries everything Run needs beyond the resolved discover.Settings.
type Options struct {
	Settings        discover.Settings
	ContinueOnError bool
	Logger          *log.Logger
}

// Run discovers groups, merges, and writes each output file. When
// ContinueOnError is true, a single failing group is logged and the
// walk continues; Run then returns a non-nil error at the end if any
// group failed. Otherwise, Run returns on the first error.
func Run(opts Options) error {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	groups, inactive, err := discover.Walk(opts.Settings)
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	for _, stem := range inactive {
		opts.Logger.Infof("skipping %s (no active layers)", stem)
	}
	if len(groups) == 0 {
		opts.Logger.Infof("no overlay files found in %s", sourceSummary(opts.Settings))
		return nil
	}

	var failed int
	for _, g := range groups {
		if err := renderGroup(g, opts.Logger); err != nil {
			if opts.ContinueOnError {
				opts.Logger.Errorf("render %s: %v", g.TargetPath, err)
				failed++
				continue
			}
			return fmt.Errorf("render %s: %w", g.TargetPath, err)
		}
	}
	succeeded := len(groups) - failed
	opts.Logger.Infof("overlayed %d %s", succeeded, pluralize(succeeded, "file", "files"))
	if failed > 0 {
		return fmt.Errorf("%d %s failed to render", failed, pluralize(failed, "file", "files"))
	}
	return nil
}

func sourceSummary(settings discover.Settings) string {
	return strings.Join(settings.SourceDirs, ", ")
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func renderGroup(g discover.Group, logger *log.Logger) error {
	content, err := MergeGroup(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(g.TargetPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(g.TargetPath, content, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	logger.Infof("%s ← [%s]", g.TargetPath, layerNames(g))
	return nil
}

// MergeGroup loads each layer of a group, folds them with merge.Merge,
// and serializes the result. It performs no disk writes — callers use
// this from both render.Run (to disk) and diff.Run (in-memory compare).
func MergeGroup(g discover.Group) ([]byte, error) {
	var merged any = map[string]any{}
	for _, layer := range g.Layers {
		data, err := os.ReadFile(layer.Path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", layer.Path, err)
		}
		parsed, err := document.Parse(data, g.Format)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", layer.Path, err)
		}
		merged = merge.Merge(merged, parsed)
	}
	return document.Serialize(merged, g.Format)
}

func layerNames(g discover.Group) string {
	names := make([]string, len(g.Layers))
	for i, l := range g.Layers {
		names[i] = l.Profile
	}
	return strings.Join(names, ", ")
}
