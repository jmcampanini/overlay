// Package orphans detects stale targets claimed by overlay state.
package orphans

import (
	"os"
	"sort"

	"github.com/jmcampanini/overlay/internal/state"
)

// Orphan identifies a stale target and its owning source.
type Orphan struct {
	Target string
	Source string
}

// Options controls which state entries Detect judges against the active plan.
type Options struct {
	Entries         []state.Entry
	PlanTargets     map[string]struct{}
	SelectedSources map[string]struct{}
	Narrowed        bool
}

// Detect returns judged regular-file entries absent from the active plan.
func Detect(opts Options) []Orphan {
	var orphans []Orphan
	for _, entry := range opts.Entries {
		if opts.Narrowed {
			if _, selected := opts.SelectedSources[entry.Source]; !selected {
				continue
			}
		}
		if _, planned := opts.PlanTargets[entry.Target]; planned {
			continue
		}
		info, err := os.Lstat(entry.Target)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		orphans = append(orphans, Orphan{Target: entry.Target, Source: entry.Source})
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].Target < orphans[j].Target
	})
	return orphans
}
