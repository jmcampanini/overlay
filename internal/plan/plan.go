// Package plan renders the dry-run view of an overlay run.
package plan

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"charm.land/log/v2"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/render"
	"github.com/jmcampanini/overlay/internal/rendermode"
	"github.com/jmcampanini/overlay/internal/substitute"
)

// Options controls plan rendering.
type Options struct {
	RenderRules       []config.RenderRule
	TOMLIndentTables  bool
	Substituter       *substitute.Resolver
	SubstituteExclude discover.Ignorer
	Logger            *log.Logger
}

// Render writes an aligned table of groups using default rendering options.
func Render(w io.Writer, groups []discover.Group, profiles []string, sourceDirs []string, targetDir string) error {
	return RenderWithOptions(w, groups, profiles, sourceDirs, targetDir, Options{})
}

// RenderWithOptions writes an aligned table with columns TARGET, MODE, and
// LAYERS, plus VARS when substitution is enabled. Substituting targets are
// composed in memory so consumed and missing variables can be reported; the
// returned error aggregates every substituting target with missing variables
// or compose failures. Non-substituting targets are not composed, so parse
// errors there surface only at render time.
func RenderWithOptions(w io.Writer, groups []discover.Group, profiles []string, sourceDirs []string, targetDir string, opts Options) error {
	if err := config.ValidateRenderRules(opts.RenderRules); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Active profiles: [%s]\n", strings.Join(profiles, ", ")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Sources: %s  Target: %s\n\n", sourceSummary(sourceDirs), collapseHome(targetDir)); err != nil {
		return err
	}

	headerStyle := lipgloss.NewStyle().Bold(true)
	cellStyle := lipgloss.NewStyle().Padding(0, 1)

	substituting := opts.Substituter.Enabled()
	headers := []string{"TARGET", "MODE", "LAYERS"}
	if substituting {
		headers = append(headers, "VARS")
	}

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle.Padding(0, 1)
			}
			return cellStyle
		}).
		Headers(headers...)

	mergeOptions := render.MergeOptions{
		TOMLIndentTables:  opts.TOMLIndentTables,
		RenderRules:       opts.RenderRules,
		TargetDir:         targetDir,
		Substituter:       opts.Substituter,
		SubstituteExclude: opts.SubstituteExclude,
	}
	var failed render.ComposeFailures
	for _, g := range groups {
		decision, err := rendermode.Decide(g, targetDir, opts.RenderRules, substituting, opts.SubstituteExclude)
		if err != nil {
			return err
		}
		row := []string{collapseHome(g.TargetPath), decision.Mode.String(), layerDisplay(g, decision.Mode)}
		if substituting {
			cell := "—"
			if decision.Substitute {
				cg := render.ComposeGroup(g, mergeOptions)
				cell = varsDisplay(cg)
				if cg.Err != nil {
					failed = append(failed, cg)
				}
			}
			row = append(row, cell)
		}
		t.Row(row...)
	}

	if _, err := fmt.Fprintln(w, t.Render()); err != nil {
		return err
	}
	noun := "files"
	if len(groups) == 1 {
		noun = "file"
	}
	if _, err := fmt.Fprintf(w, "\n%d %s will be generated\n", len(groups), noun); err != nil {
		return err
	}
	render.WarnUnusedPins(opts.Substituter, failed, opts.Logger)
	if len(failed) > 0 {
		return failed
	}
	return nil
}

// varsDisplay renders one target's VARS cell: consumed names with missing
// ones marked, or a compose-error note when the target could not compose at
// all.
func varsDisplay(cg render.ComposedGroup) string {
	var missingErr *render.MissingVarsError
	if cg.Err != nil && !errors.As(cg.Err, &missingErr) {
		return "(compose error)"
	}
	if len(cg.Vars.Consumed) == 0 {
		return ""
	}
	missing := make(map[string]struct{}, len(cg.Vars.Missing))
	for _, name := range cg.Vars.Missing {
		missing[name] = struct{}{}
	}
	parts := make([]string, len(cg.Vars.Consumed))
	for i, name := range cg.Vars.Consumed {
		if _, ok := missing[name]; ok {
			parts[i] = name + " (missing!)"
		} else {
			parts[i] = name
		}
	}
	return strings.Join(parts, ", ")
}

func layerDisplay(g discover.Group, mode rendermode.Mode) string {
	layers := make([]string, len(g.Layers))
	for i, l := range g.Layers {
		layers[i] = l.Profile
	}
	display := strings.Join(layers, ", ")
	if mode == rendermode.ModeCopy && len(layers) > 0 {
		display += " (winner: " + layers[len(layers)-1] + ")"
	}
	return display
}

func sourceSummary(sources []string) string {
	if len(sources) == 0 {
		return "."
	}
	if len(sources) > 4 {
		return fmt.Sprintf("%d configured", len(sources))
	}
	out := make([]string, len(sources))
	for i, source := range sources {
		out[i] = collapseHome(source)
	}
	return strings.Join(out, ", ")
}

// collapseHome shortens an absolute path to start with ~/ when it begins
// with the current user's home directory, or returns "~" when the path
// is exactly the home directory.
func collapseHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if after, ok := strings.CutPrefix(p, home+string(os.PathSeparator)); ok {
		return "~/" + after
	}
	return p
}
