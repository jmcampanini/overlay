package config

import (
	"fmt"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
)

// reservedProfiles cannot appear in the profiles list — they name the
// special base and local layers.
var reservedProfiles = []string{"base", "local"}

// Validate checks the in-memory Config for Overlay-specific schema problems.
// Target is not required by this method because env/flag/runtime resolution can
// supply it later; the CLI resolver checks the final value.
func (c Config) Validate() error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("sources must contain at least one source directory")
	}
	for _, source := range c.Sources {
		if strings.TrimSpace(source) == "" {
			return fmt.Errorf("sources contains an empty source directory")
		}
	}
	if err := ValidateProfiles(c.Profiles); err != nil {
		return err
	}
	return ValidateRenderRules(c.RenderRules)
}

// ValidateProfiles checks profile names after env_profiles has been applied.
func ValidateProfiles(profiles []string) error {
	for _, p := range profiles {
		if slices.Contains(reservedProfiles, p) {
			return fmt.Errorf("profile name %q is reserved (base and local are special layers)", p)
		}
	}
	return nil
}

// ValidateRenderRules checks render rule paths, strategies, and duplicates.
func ValidateRenderRules(rules []RenderRule) error {
	seen := make(map[string]int, len(rules))
	for i, rule := range rules {
		normalized, err := NormalizeRenderRulePath(rule.Path)
		if err != nil {
			return fmt.Errorf("render_rules[%d].path: %w", i, err)
		}
		switch rule.Strategy {
		case RenderStrategyAppend, RenderStrategyCopy:
		case "":
			return fmt.Errorf("render_rules[%d].strategy is required", i)
		default:
			return fmt.Errorf("render_rules[%d].strategy %q is unsupported (supported: append, copy)", i, rule.Strategy)
		}
		if prev, ok := seen[normalized]; ok {
			return fmt.Errorf("render_rules[%d].path duplicates render_rules[%d].path %q", i, prev, normalized)
		}
		seen[normalized] = i
	}
	return nil
}

// NormalizeRenderRulePath returns the slash-separated path key used for matching.
func NormalizeRenderRulePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(p) || pathpkg.IsAbs(filepath.ToSlash(p)) {
		return "", fmt.Errorf("path must be target-relative")
	}
	slash := filepath.ToSlash(p)
	for _, part := range strings.Split(slash, "/") {
		if part == ".." {
			return "", fmt.Errorf("path must not contain '..'")
		}
	}
	return pathpkg.Clean(slash), nil
}

// ValidateFile parses the file at path and reports schema problems as an
// error. A missing file is an error. Unlike Config.Validate, this also requires
// Target to be set, since the only way to fix it from a file is to edit the
// file.
func ValidateFile(path string) error {
	c, _, err := LoadRequired(path)
	if err != nil {
		return err
	}
	if c.Target == "" {
		return fmt.Errorf("%s: 'target' is required", path)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
