package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/spf13/cobra"
)

func filePatternArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
		return err
	}
	for _, pattern := range args {
		var rule string
		switch {
		case pattern == "":
			rule = "must not be empty"
		case !strings.Contains(pattern, "{layer}"):
			rule = "must contain {layer}"
		case strings.HasPrefix(pattern, "/"), strings.HasPrefix(pattern, "~"):
			rule = "must be relative to the source root, without a leading / or ~"
		case slices.Contains(strings.Split(pattern, "/"), ".."):
			rule = "must not contain a .. path segment"
		case strings.ContainsAny(strings.ReplaceAll(pattern, "{layer}", ""), "{}"):
			rule = "must not contain braces outside {layer}"
		case strings.ContainsAny(pattern, "*?["):
			rule = "must not contain *, ?, or [; globs are reserved"
		}
		if rule != "" {
			return fmt.Errorf("pattern %q %s", pattern, rule)
		}
	}
	return nil
}

func selectFiles(cmd *cobra.Command, configPath string, patterns []string) ([]string, error) {
	loaded, err := loadConfiguration(cmd, configPath)
	if err != nil {
		return nil, err
	}
	sources := loaded.effective.Sources
	manifests, err := config.LoadManifests(sources)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, layer := range config.LayerOrder(loaded.effective.Layers) {
		matchedSources := make(map[string]string)
		for i, source := range sources {
			if _, declared := manifests[i].Layers[layer]; !declared {
				continue
			}
			for _, pattern := range patterns {
				path := filepath.Join(source.Path, strings.ReplaceAll(pattern, "{layer}", layer))
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
