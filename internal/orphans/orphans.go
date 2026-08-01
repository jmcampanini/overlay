// Package orphans detects stale targets claimed by overlay state.
package orphans

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"syscall"

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
// It fails when an owned target or potential active alias cannot be inspected.
func Detect(opts Options) ([]Orphan, error) {
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
		if err != nil {
			if pathAbsent(err) {
				continue
			}
			return nil, fmt.Errorf("inspect owned target %q: %w", entry.Target, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		aliased, err := aliasesPlanTarget(info, opts.PlanTargets)
		if err != nil {
			return nil, err
		}
		if aliased {
			continue
		}
		orphans = append(orphans, Orphan{Target: entry.Target, Source: entry.Source})
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].Target < orphans[j].Target
	})
	return orphans, nil
}

func aliasesPlanTarget(targetInfo fs.FileInfo, planTargets map[string]struct{}) (bool, error) {
	paths := make([]string, 0, len(planTargets))
	for path := range planTargets {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var inspectionErr error
	for _, planTarget := range paths {
		planInfo, err := os.Stat(planTarget)
		if err != nil {
			if pathAbsent(err) {
				continue
			}
			if inspectionErr == nil {
				inspectionErr = fmt.Errorf("inspect active target %q: %w", planTarget, err)
			}
			continue
		}
		if os.SameFile(targetInfo, planInfo) {
			return true, nil
		}
	}
	return false, inspectionErr
}

func pathAbsent(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}
