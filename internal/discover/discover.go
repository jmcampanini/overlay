// Package discover walks source directories, identifies overlay groups by
// the *.olay.*.* filename convention, and resolves each group's target
// path and ordered layer list.
package discover

import (
	"cmp"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmcampanini/overlay/internal/document"
)

const (
	// Marker is the fixed segment that identifies overlay source files:
	// <stem>.olay.<profile>.<ext>
	Marker = "olay"

	// ProfileBase is the reserved profile name for the first merge layer.
	ProfileBase = "base"
	// ProfileLocal is the reserved profile name for the last merge layer.
	ProfileLocal = "local"
)

// Settings is the resolved configuration that Walk needs.
type Settings struct {
	SourceDir        string
	SourceDirs       []string
	TargetDir        string
	DotPrefix        bool
	Profiles         []string
	Ignore           Ignorer
	TraverseHidden   bool
	RespectGitignore bool
}

// Layer represents a single source file contributing to an overlay group.
type Layer struct {
	Profile string // "base", "local", or a user profile name
	Path    string // absolute path to the source file
}

// Group is one output file's worth of overlay sources: a stem, a format,
// an ordered list of active layers, and the resolved target path.
//
// Group instances returned from Walk's active slice always have at least
// one Layer. Construct Groups manually only in tests.
type Group struct {
	SourceDir  string
	Stem       string
	Format     document.Format
	TargetPath string
	Layers     []Layer
}

// newGroup constructs a Group with the active-group invariant: Layers
// must be non-empty, Format must be a recognized format, and TargetPath
// must be non-empty.
func newGroup(sourceDir, stem string, format document.Format, targetPath string, layers []Layer) (Group, error) {
	if len(layers) == 0 {
		return Group{}, fmt.Errorf("group %q has no active layers", stem)
	}
	if format == document.FormatUnknown {
		return Group{}, fmt.Errorf("group %q has unknown format", stem)
	}
	if targetPath == "" {
		return Group{}, fmt.Errorf("group %q has empty target path", stem)
	}
	return Group{SourceDir: sourceDir, Stem: stem, Format: format, TargetPath: targetPath, Layers: layers}, nil
}

// Walk scans s.SourceDirs for overlay groups. If SourceDirs is empty, SourceDir
// is used for backward compatibility with tests and direct package callers. The
// active slice contains only groups whose layer list is non-empty (the invariant
// pinned by newGroup). The inactive slice contains the stems of groups that
// matched the file convention but had no active profile layer; the caller should
// log these for visibility.
func Walk(s Settings) ([]Group, []string, error) {
	dirs := effectiveSourceDirs(s)
	if len(dirs) == 0 {
		return nil, nil, fmt.Errorf("source directories are empty")
	}

	var active []Group
	var inactive []string
	for _, source := range dirs {
		if source == "" {
			return nil, nil, fmt.Errorf("source directory is empty")
		}
		absSource, err := filepath.Abs(source)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve source %q: %w", source, err)
		}
		groups, skipped, err := walkSource(s, absSource)
		if err != nil {
			return nil, nil, err
		}
		active = append(active, groups...)
		inactive = append(inactive, skipped...)
	}

	slices.SortFunc(active, func(a, b Group) int {
		return cmp.Or(
			cmp.Compare(a.SourceDir, b.SourceDir),
			cmp.Compare(a.TargetPath, b.TargetPath),
			cmp.Compare(a.Stem, b.Stem),
			cmp.Compare(a.Format.String(), b.Format.String()),
		)
	})

	seenTargets := make(map[string]string, len(active))
	for _, g := range active {
		source := g.Layers[0].Path
		if prev, ok := seenTargets[g.TargetPath]; ok {
			return nil, nil, fmt.Errorf(
				"target path collision: %q is produced by both %q and %q",
				g.TargetPath, prev, source,
			)
		}
		seenTargets[g.TargetPath] = source
	}

	return active, inactive, nil
}

func effectiveSourceDirs(s Settings) []string {
	if len(s.SourceDirs) > 0 {
		return append([]string(nil), s.SourceDirs...)
	}
	if s.SourceDir != "" {
		return []string{s.SourceDir}
	}
	return nil
}

func walkSource(s Settings, absSource string) ([]Group, []string, error) {
	ignorer := s.Ignore
	if ignorer == nil {
		ignorer = NoopIgnorer()
	}
	if s.RespectGitignore {
		gitignoreIgn, err := NewGitignoreIgnorer(absSource)
		if err != nil {
			return nil, nil, err
		}
		ignorer = NewChain(ignorer, gitignoreIgn)
	}

	type key struct {
		source string
		relDir string
		stem   string
		ext    string
	}
	type discovered struct {
		stem       string
		format     document.Format
		targetPath string
	}
	groups := make(map[key]discovered)
	layerSources := make(map[key]map[string]string) // all discovered layers, keyed by profile

	walkErr := filepath.WalkDir(absSource, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(absSource, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if !s.TraverseHidden && isHiddenDir(d.Name()) {
				return filepath.SkipDir
			}
			if ignorer.Match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignorer.Match(rel, false) {
			return nil
		}
		stem, profile, ext, ok := ParseOverlayName(d.Name())
		if !ok {
			return nil
		}
		format, formatErr := document.DetectFormat("f." + ext)
		if formatErr != nil {
			return nil
		}
		relDir, relErr := filepath.Rel(absSource, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		if relDir == "." {
			relDir = ""
		}
		k := key{source: absSource, relDir: relDir, stem: stem, ext: ext}
		if _, exists := layerSources[k]; !exists {
			layerSources[k] = make(map[string]string)
		}
		layerSources[k][profile] = path

		if _, exists := groups[k]; !exists {
			target, terr := TargetPath(relDir, stem, ext, s.TargetDir, s.DotPrefix)
			if terr != nil {
				return terr
			}
			groups[k] = discovered{stem: stem, format: format, targetPath: target}
		}
		return nil
	})
	if walkErr != nil {
		return nil, nil, walkErr
	}

	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b key) int {
		return cmp.Or(
			cmp.Compare(a.source, b.source),
			cmp.Compare(a.relDir, b.relDir),
			cmp.Compare(a.stem, b.stem),
			cmp.Compare(a.ext, b.ext),
		)
	})

	active := make([]Group, 0, len(keys))
	var inactive []string
	for _, k := range keys {
		d := groups[k]
		layers := orderedLayers(layerSources[k], s.Profiles)
		if len(layers) == 0 {
			inactive = append(inactive, d.stem)
			continue
		}
		g, err := newGroup(absSource, d.stem, d.format, d.targetPath, layers)
		if err != nil {
			return nil, nil, err
		}
		active = append(active, g)
	}
	return active, inactive, nil
}

// orderedLayers picks the active layers from the discovered set and orders
// them as base -> profiles in order -> local.
func orderedLayers(discovered map[string]string, profiles []string) []Layer {
	out := make([]Layer, 0, len(discovered))
	if path, ok := discovered[ProfileBase]; ok {
		out = append(out, Layer{Profile: ProfileBase, Path: path})
	}
	seen := map[string]bool{ProfileBase: true, ProfileLocal: true}
	for _, p := range profiles {
		if seen[p] {
			continue
		}
		seen[p] = true
		if path, ok := discovered[p]; ok {
			out = append(out, Layer{Profile: p, Path: path})
		}
	}
	if path, ok := discovered[ProfileLocal]; ok {
		out = append(out, Layer{Profile: ProfileLocal, Path: path})
	}
	return out
}

// ParseOverlayName parses a filename of the form <stem>.olay.<profile>.<ext>.
// It returns (stem, profile, ext, true) on success and zero values + false
// on any non-match. The stem may itself contain dots.
func ParseOverlayName(name string) (stem, profile, ext string, ok bool) {
	parts := strings.Split(name, ".")
	if len(parts) < 4 {
		return "", "", "", false
	}
	ext = parts[len(parts)-1]
	profile = parts[len(parts)-2]
	marker := parts[len(parts)-3]
	if marker != Marker {
		return "", "", "", false
	}
	stem = strings.Join(parts[:len(parts)-3], ".")
	if stem == "" || profile == "" {
		return "", "", "", false
	}
	switch ext {
	case "json", "toml":
		return stem, profile, ext, true
	}
	return "", "", "", false
}

func isHiddenDir(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}
