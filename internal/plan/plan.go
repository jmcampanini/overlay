// Package plan renders the dry-run view of an overlay run.
package plan

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/jmcampanini/overlay/internal/discover"
)

// Render writes an aligned table of groups to w, with columns
// TARGET, FORMAT, LAYERS. The header summarizes the active profile
// set and source/target directories.
func Render(w io.Writer, groups []discover.Group, profiles []string, sourceDir, targetDir string) error {
	if _, err := fmt.Fprintf(w, "Active profiles: [%s]\n", strings.Join(profiles, ", ")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source: %s  Target: %s\n\n", sourceDir, collapseHome(targetDir)); err != nil {
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
		Headers("TARGET", "FORMAT", "LAYERS")

	for _, g := range groups {
		layers := make([]string, len(g.Layers))
		for i, l := range g.Layers {
			layers[i] = l.Profile
		}
		t.Row(collapseHome(g.TargetPath), g.Format.String(), strings.Join(layers, ", "))
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
