package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/jmcampanini/overlay/internal/render"
)

// planDocument is the --json shape of a render plan.
type planDocument struct {
	Target  string      `json:"target"`
	Layers  []string    `json:"layers"`
	Entries []planEntry `json:"entries"`
}

type planEntry struct {
	Target   string       `json:"target"`
	Source   string       `json:"source"`
	Path     string       `json:"path"`
	Layer    string       `json:"layer"`
	Kind     string       `json:"kind"`
	Status   string       `json:"status"`
	Replaces []planOrigin `json:"replaces"`
	Merges   []planOrigin `json:"merges"`
}

type planOrigin struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	Layer  string `json:"layer"`
}

func writePlanJSON(out io.Writer, plan render.Plan) error {
	doc := planDocument{Target: plan.Target, Layers: plan.Layers, Entries: make([]planEntry, 0, len(plan.Entries))}
	for _, entry := range plan.Entries {
		merges := make([]planOrigin, 0, len(entry.Merges))
		for _, fragment := range entry.Merges {
			merges = append(merges, planOriginOf(fragment.Origin))
		}
		replaces := make([]planOrigin, 0, len(entry.Replaces))
		for _, origin := range entry.Replaces {
			replaces = append(replaces, planOriginOf(origin))
		}
		doc.Entries = append(doc.Entries, planEntry{
			Target:   entry.Target,
			Source:   entry.Source,
			Path:     entry.Path,
			Layer:    entry.Layer,
			Kind:     string(entry.Kind),
			Status:   string(entry.Status),
			Replaces: replaces,
			Merges:   merges,
		})
	}

	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(doc)
}

func planOriginOf(origin render.Origin) planOrigin {
	return planOrigin{Source: origin.Source, Path: origin.Path, Layer: origin.Layer}
}

// writePlanTable prints one row per entry and a count summary. home, when
// nonempty, is shortened to ~ in target paths. Styles apply only when color
// is on, so a redirected run carries no escapes.
func writePlanTable(out io.Writer, plan render.Plan, home string, color bool) error {
	plain := lipgloss.NewStyle()
	header := plain
	statusStyles := map[render.Status]lipgloss.Style{}
	if color {
		header = plain.Bold(true)
		statusStyles[render.StatusNew] = plain.Foreground(lipgloss.Green)
		statusStyles[render.StatusChanged] = plain.Foreground(lipgloss.Yellow)
	}

	counts := map[render.Status]int{}
	rows := make([][]string, 0, len(plan.Entries))
	for _, entry := range plan.Entries {
		counts[entry.Status]++
		status := string(entry.Status)
		if style, ok := statusStyles[entry.Status]; ok {
			status = style.Render(status)
		}
		rows = append(rows, []string{
			collapseHome(entry.Target, home),
			sourceCell(entry),
			string(entry.Kind),
			status,
			replacesCell(entry.Replaces),
		})
	}

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return header.PaddingRight(2)
			}
			return plain.PaddingRight(2)
		}).
		Headers("TARGET", "SOURCE", "KIND", "STATUS", "REPLACES").
		Rows(rows...)

	var lines strings.Builder
	for _, line := range strings.Split(t.String(), "\n") {
		lines.WriteString(strings.TrimRight(line, " "))
		lines.WriteString("\n")
	}
	noun := "files"
	if len(plan.Entries) == 1 {
		noun = "file"
	}
	fmt.Fprintf(&lines, "\n%d %s: %d new, %d changed, %d unchanged\n",
		len(plan.Entries), noun, counts[render.StatusNew], counts[render.StatusChanged], counts[render.StatusUnchanged])

	_, err := io.WriteString(out, lines.String())
	return err
}

// sourceCell names the file an entry renders from, or for a merge every
// fragment in layer order.
func sourceCell(entry render.Entry) string {
	if entry.Kind != render.KindMerge {
		return entry.Source + ":" + entry.Path
	}
	parts := make([]string, len(entry.Merges))
	for i, fragment := range entry.Merges {
		parts[i] = fragment.Source + ":" + fragment.Path
	}
	return strings.Join(parts, ", ")
}

func replacesCell(origins []render.Origin) string {
	parts := make([]string, len(origins))
	for i, origin := range origins {
		parts[i] = fmt.Sprintf("%s:%s (%s)", origin.Source, origin.Path, origin.Layer)
	}
	return strings.Join(parts, ", ")
}

// collapseHome shortens path to ~ or ~/rest when it lies under home.
func collapseHome(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+"/"); ok {
		return "~/" + rest
	}
	return path
}
