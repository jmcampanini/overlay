package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// layerPlaceholder is the token every file pattern holds; each occurrence
// is replaced with a layer name.
const layerPlaceholder = "{layer}"

// ValidatePattern reports why pattern cannot select source files: it must
// be nonempty, hold {layer}, be relative to the source root without a
// leading / or ~, hold no .. segment, hold no braces outside {layer}, and
// hold none of *, ?, or [, which are reserved for globs.
func ValidatePattern(pattern string) error {
	var rule string
	switch {
	case pattern == "":
		rule = "must not be empty"
	case !strings.Contains(pattern, layerPlaceholder):
		rule = "must contain {layer}"
	case strings.HasPrefix(pattern, "/"), strings.HasPrefix(pattern, "~"):
		rule = "must be relative to the source root, without a leading / or ~"
	case slices.Contains(strings.Split(pattern, "/"), ".."):
		rule = "must not contain a .. path segment"
	case strings.ContainsAny(strings.ReplaceAll(pattern, layerPlaceholder, ""), "{}"):
		rule = "must not contain braces outside {layer}"
	case strings.ContainsAny(pattern, "*?["):
		rule = "must not contain *, ?, or [; globs are reserved"
	}
	if rule != "" {
		return fmt.Errorf("pattern %q %s", pattern, rule)
	}
	return nil
}

// SelectFiles returns the absolute path of every file the patterns select,
// ordered by layer, then source, then pattern. manifests[i] is the manifest
// of sources[i]. A pattern matches in a source only for layers that
// source's manifest declares; a regular file or a symbolic link to one
// matches, and anything else, including an absent path, contributes
// nothing. A malformed pattern, a stat error other than absence, or one
// pattern matching in two sources for the same layer is an error. Files
// are not deduplicated.
func SelectFiles(sources []ResolvedSource, manifests []Manifest, layers []string, patterns []string) ([]string, error) {
	for _, pattern := range patterns {
		if err := ValidatePattern(pattern); err != nil {
			return nil, err
		}
	}

	var paths []string
	for _, layer := range layers {
		matchedSources := make(map[string]string)
		for i, source := range sources {
			if _, declared := manifests[i].Layers[layer]; !declared {
				continue
			}
			for _, pattern := range patterns {
				path := filepath.Join(source.Path, strings.ReplaceAll(pattern, layerPlaceholder, layer))
				info, err := os.Stat(path)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return nil, fmt.Errorf("source %s: stat %q: %w", source.Name, path, err)
				}
				if !info.Mode().IsRegular() {
					continue
				}
				if previous, ok := matchedSources[pattern]; ok && previous != source.Name {
					return nil, fmt.Errorf("%s: layer %s matches in two sources: %s, %s", pattern, layer, previous, source.Name)
				}
				matchedSources[pattern] = source.Name
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}
