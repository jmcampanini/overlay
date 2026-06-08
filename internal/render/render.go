// Package render orchestrates the full overlay pipeline: discover groups,
// merge their layers, serialize, and write the outputs to disk.
package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/log/v2"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
	"github.com/jmcampanini/overlay/internal/merge"
	"github.com/jmcampanini/overlay/internal/rendermode"
)

// Options carries everything Run needs beyond the resolved discover.Settings.
type Options struct {
	Settings         discover.Settings
	ContinueOnError  bool
	TOMLIndentTables bool
	RenderRules      []config.RenderRule
	Logger           *log.Logger
}

// MergeOptions controls output formatting for merged groups.
type MergeOptions struct {
	TOMLIndentTables bool
	RenderRules      []config.RenderRule
	TargetDir        string
}

// Run discovers groups, merges, and writes each output file. When
// ContinueOnError is true, a single failing group is logged and the
// walk continues; Run then returns a non-nil error at the end if any
// group failed. Otherwise, Run returns on the first error.
func Run(opts Options) error {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if err := config.ValidateRenderRules(opts.RenderRules); err != nil {
		return err
	}
	result, err := discover.WalkDetailed(opts.Settings)
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	for _, source := range result.MissingSources {
		opts.Logger.Warnf("source %q not found, skipping", source)
	}
	for _, stem := range result.Inactive {
		opts.Logger.Infof("skipping %s (no active layers)", stem)
	}
	groups := result.Active
	if len(groups) == 0 {
		opts.Logger.Debugf("no overlay files found in %s", sourceSummary(opts.Settings))
		return nil
	}

	var failed int
	mergeOptions := MergeOptions{
		TOMLIndentTables: opts.TOMLIndentTables,
		RenderRules:      opts.RenderRules,
		TargetDir:        opts.Settings.TargetDir,
	}
	for _, g := range groups {
		if err := renderGroup(g, opts.Logger, mergeOptions); err != nil {
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

func renderGroup(g discover.Group, logger *log.Logger, opts MergeOptions) error {
	content, err := MergeGroupWithOptions(g, opts)
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

// MergeGroup loads each layer of a structured group, folds them with
// merge.Merge, and serializes the result. Copy-through groups return the
// winning layer's bytes and append groups concatenate active layers. It performs
// no disk writes — callers use this from both render.Run and diff.Run.
func MergeGroup(g discover.Group) ([]byte, error) {
	return MergeGroupWithOptions(g, MergeOptions{})
}

// MergeGroupWithOptions is MergeGroup with output formatting options.
func MergeGroupWithOptions(g discover.Group, opts MergeOptions) ([]byte, error) {
	mode, err := rendermode.ForGroup(g, opts.TargetDir, opts.RenderRules)
	if err != nil {
		return nil, err
	}
	switch mode {
	case rendermode.ModeCopy:
		return copyWinningLayer(g)
	case rendermode.ModeAppend:
		return appendLayers(g)
	}

	var merged any = map[string]any{}
	for _, layer := range g.Layers {
		data, err := readLayer(layer)
		if err != nil {
			return nil, err
		}
		parsed, err := document.Parse(data, g.Format)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", layer.Path, err)
		}
		merged = merge.Merge(merged, parsed)
	}
	return document.SerializeWithOptions(merged, g.Format, document.SerializeOptions{
		TOMLIndentTables: opts.TOMLIndentTables,
	})
}

func readLayer(layer discover.Layer) ([]byte, error) {
	data, err := os.ReadFile(layer.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", layer.Path, err)
	}
	return data, nil
}

func copyWinningLayer(g discover.Group) ([]byte, error) {
	if len(g.Layers) == 0 {
		return nil, fmt.Errorf("copy group %q has no active layers", g.Stem)
	}
	return readLayer(g.Layers[len(g.Layers)-1])
}

func appendLayers(g discover.Group) ([]byte, error) {
	if len(g.Layers) == 0 {
		return nil, fmt.Errorf("append group %q has no active layers", g.Stem)
	}
	var out []byte
	for _, layer := range g.Layers {
		data, err := readLayer(layer)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, data...)
	}
	return out, nil
}

func layerNames(g discover.Group) string {
	names := make([]string, len(g.Layers))
	for i, l := range g.Layers {
		names[i] = l.Profile
	}
	return strings.Join(names, ", ")
}
