// Package plan renders the dry-run view of an overlay run.
package plan

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/rendermode"
)

// Options controls plan rendering.
type Options struct {
	RenderRules []config.RenderRule
}

// Render writes an aligned table of groups using default rendering options.
func Render(w io.Writer, groups []discover.Group, profiles []string, sourceDirs []string, targetDir string) error {
	return RenderWithOptions(w, groups, profiles, sourceDirs, targetDir, Options{})
}

// RenderWithOptions writes an aligned table with columns TARGET, MODE, LAYERS.
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

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle.Padding(0, 1)
			}
			return cellStyle
		}).
		Headers("TARGET", "MODE", "LAYERS")

	for _, g := range groups {
		mode, err := rendermode.ForGroup(g, targetDir, opts.RenderRules)
		if err != nil {
			return err
		}
		t.Row(collapseHome(g.TargetPath), mode.String(), layerDisplay(g, mode))
	}

	if _, err := fmt.Fprintln(w, t.Render()); err != nil {
		return err
	}
	noun := "files"
	if len(groups) == 1 {
		noun = "file"
	}
	_, err := fmt.Fprintf(w, "\n%d %s will be generated\n", len(groups), noun)
	return err
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
