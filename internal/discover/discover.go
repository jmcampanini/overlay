// Package discover walks source directories, identifies overlay groups by
// the *.olay.*[.*] filename convention, and resolves each group's target
// path and ordered layer list.
package discover

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmcampanini/overlay/internal/document"
)

const (
	// Marker is the fixed segment that identifies overlay source files:
	// <stem>.olay.<profile>[.<ext>]
	Marker = "olay"

	// ProfileBase is the reserved profile name for the first merge layer.
	ProfileBase = "base"
	// ProfileLocal is the reserved profile name for the last merge layer.
	ProfileLocal = "local"
)

// Settings is the resolved configuration that Walk needs.
type Settings struct {
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

// WalkResult is the complete result of scanning source directories.
type WalkResult struct {
	Active         []Group
	Inactive       []string
	MissingSources []string
}

// Walk scans s.SourceDirs for overlay groups. The active slice contains only
// groups whose layer list is non-empty (the invariant
// pinned by newGroup). The inactive slice contains the stems of groups that
// matched the file convention but had no active profile layer; the caller should
// log these for visibility.
func Walk(s Settings) ([]Group, []string, error) {
	result, err := WalkDetailed(s)
	if err != nil {
		return nil, nil, err
	}
	return result.Active, result.Inactive, nil
}

// WalkDetailed is Walk plus observability metadata for skipped source roots.
func WalkDetailed(s Settings) (WalkResult, error) {
	dirs := effectiveSourceDirs(s)
	if len(dirs) == 0 {
		return WalkResult{}, fmt.Errorf("source directories are empty")
	}

	var result WalkResult
	for _, source := range dirs {
		if source == "" {
			return WalkResult{}, fmt.Errorf("source directory is empty")
		}
		absSource, err := filepath.Abs(source)
		if err != nil {
			return WalkResult{}, fmt.Errorf("resolve source %q: %w", source, err)
		}
		exists, err := sourceExists(absSource)
		if err != nil {
			return WalkResult{}, err
		}
		if !exists {
			result.MissingSources = append(result.MissingSources, absSource)
			continue
		}
		groups, skipped, err := walkSource(s, absSource)
		if err != nil {
			return WalkResult{}, err
		}
		result.Active = append(result.Active, groups...)
		result.Inactive = append(result.Inactive, skipped...)
	}

	slices.SortFunc(result.Active, func(a, b Group) int {
		return cmp.Or(
			cmp.Compare(a.SourceDir, b.SourceDir),
			cmp.Compare(a.TargetPath, b.TargetPath),
			cmp.Compare(a.Stem, b.Stem),
			cmp.Compare(a.Format.String(), b.Format.String()),
		)
	})

	seenTargets := make(map[string]string, len(result.Active))
	for _, g := range result.Active {
		source := g.Layers[0].Path
		if prev, ok := seenTargets[g.TargetPath]; ok {
			return WalkResult{}, fmt.Errorf(
				"target path collision: %q is produced by both %q and %q",
				g.TargetPath, prev, source,
			)
		}
		seenTargets[g.TargetPath] = source
	}

	return result, nil
}

func effectiveSourceDirs(s Settings) []string {
	if len(s.SourceDirs) == 0 {
		return nil
	}
	return append([]string(nil), s.SourceDirs...)
}

func sourceExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat source %q: %w", path, err)
	}
	return true, nil
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
		relDir string
		stem   string
		ext    string
	}
	type groupInfo struct {
		format     document.Format
		targetPath string
	}
	groups := make(map[key]groupInfo)
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
		stem, profile, ext, ok, parseErr := ParseOverlayNameStrict(d.Name())
		if parseErr != nil {
			return fmt.Errorf("invalid overlay filename %q: %w", rel, parseErr)
		}
		if !ok {
			return nil
		}
		format := formatForExtension(ext)
		relDir, relErr := filepath.Rel(absSource, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		if relDir == "." {
			relDir = ""
		}
		k := key{relDir: relDir, stem: stem, ext: ext}
		if _, exists := layerSources[k]; !exists {
			layerSources[k] = make(map[string]string)
		}
		layerSources[k][profile] = path

		if _, exists := groups[k]; !exists {
			target, terr := TargetPath(relDir, stem, ext, s.TargetDir, s.DotPrefix)
			if terr != nil {
				return terr
			}
			groups[k] = groupInfo{format: format, targetPath: target}
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
			inactive = append(inactive, k.stem)
			continue
		}
		g, err := newGroup(absSource, k.stem, d.format, d.targetPath, layers)
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

// ParseOverlayName parses a filename of the form
// <stem>.olay.<profile>[.<ext>]. It returns (stem, profile, ext, true) on
// success and zero values + false on any non-match or malformed match. The
// stem may itself contain dots.
func ParseOverlayName(name string) (stem, profile, ext string, ok bool) {
	stem, profile, ext, ok, _ = ParseOverlayNameStrict(name)
	return stem, profile, ext, ok
}

// ParseOverlayNameStrict parses a filename of the form
// <stem>.olay.<profile>[.<ext>]. It returns ok=false when the marker is absent
// and a non-nil error when the filename contains the marker but is malformed.
func ParseOverlayNameStrict(name string) (stem, profile, ext string, ok bool, err error) {
	parts := strings.Split(name, ".")
	if len(parts) >= 4 {
		marker := len(parts) - 3
		if parts[marker] == Marker {
			stem = strings.Join(parts[:marker], ".")
			return validateOverlayParts(stem, parts[marker+1], parts[marker+2], true)
		}
	}
	if len(parts) >= 3 {
		marker := len(parts) - 2
		if parts[marker] == Marker {
			stem = strings.Join(parts[:marker], ".")
			return validateOverlayParts(stem, parts[marker+1], "", false)
		}
	}
	for i, part := range parts {
		if part != Marker {
			continue
		}
		tail := len(parts) - i - 1
		switch {
		case i == 0:
			return "", "", "", false, fmt.Errorf("missing stem")
		case tail == 0:
			return "", "", "", false, fmt.Errorf("missing profile")
		case tail > 2:
			return "", "", "", false, fmt.Errorf("multi-part extension after profile")
		}
	}
	return "", "", "", false, nil
}

func validateOverlayParts(stem, profile, ext string, extPresent bool) (string, string, string, bool, error) {
	switch {
	case stem == "":
		return "", "", "", false, fmt.Errorf("missing stem")
	case profile == "":
		return "", "", "", false, fmt.Errorf("missing profile")
	case extPresent && ext == "":
		return "", "", "", false, fmt.Errorf("missing extension")
	}
	return stem, profile, ext, true, nil
}

func formatForExtension(ext string) document.Format {
	switch strings.ToLower(ext) {
	case "json":
		return document.FormatJSON
	case "toml":
		return document.FormatTOML
	}
	return document.FormatCopy
}

func isHiddenDir(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}
