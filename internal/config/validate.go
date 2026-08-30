package config

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmcampanini/overlay/internal/substitute"
)

// reservedProfiles cannot appear in the profiles list - they name the
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
	if err := ValidateEnvProfiles(c.EnvProfiles); err != nil {
		return err
	}
	if err := ValidateProfiles(c.Profiles); err != nil {
		return err
	}
	if err := ValidateRenderRules(c.RenderRules); err != nil {
		return err
	}
	return ValidateSubstitutePrefixes(c.SubstitutePrefixes)
}

// ValidateSubstitutePrefixes checks that every prefix entry is a non-empty
// POSIX-shaped name fragment. An empty entry would match every variable,
// silently turning prefix gating into substitute-everything.
func ValidateSubstitutePrefixes(prefixes []string) error {
	for i, p := range prefixes {
		if !substitute.ValidName(p) {
			return fmt.Errorf("substitute_prefixes[%d] %q must match [A-Za-z_][A-Za-z0-9_]*", i, p)
		}
	}
	return nil
}

// ValidateEnvProfiles rejects blank or whitespace-padded environment variable
// names in env_profiles. Padded names must error rather than be trimmed:
// os.Getenv looks up the exact string, so a padded name would silently never
// match.
func ValidateEnvProfiles(names []string) error {
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return fmt.Errorf("env_profiles contains an empty environment variable name")
		}
		if name != trimmed {
			return fmt.Errorf("env_profiles entry %q has leading or trailing whitespace", name)
		}
	}
	return nil
}

// ValidateProfiles rejects the reserved profile names base and local. Callers
// run it on both the raw TOML list and the effective list after env_profiles
// is applied.
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
	_, err := NormalizeRenderRules(rules)
	return err
}

// NormalizeRenderRules validates rules and returns a copy with normalized paths.
func NormalizeRenderRules(rules []RenderRule) ([]RenderRule, error) {
	normalized := make([]RenderRule, 0, len(rules))
	seen := make(map[string]int, len(rules))
	for i, rule := range rules {
		normalizedPath, err := NormalizeRenderRulePath(rule.Path)
		if err != nil {
			return nil, fmt.Errorf("render_rules[%d].path: %w", i, err)
		}
		switch rule.Strategy {
		case RenderStrategyAppend, RenderStrategyCopy:
		case "":
			return nil, fmt.Errorf("render_rules[%d].strategy is required", i)
		default:
			return nil, fmt.Errorf("render_rules[%d].strategy %q is unsupported (supported: append, copy)", i, rule.Strategy)
		}
		if prev, ok := seen[normalizedPath]; ok {
			return nil, fmt.Errorf("render_rules[%d].path duplicates render_rules[%d].path %q", i, prev, normalizedPath)
		}
		seen[normalizedPath] = i
		normalized = append(normalized, RenderRule{Path: normalizedPath, Strategy: rule.Strategy})
	}
	return normalized, nil
}

// NormalizeRenderRulePath returns the slash-separated path key used for matching.
func NormalizeRenderRulePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("path is required")
	}
	slash := filepath.ToSlash(p)
	if filepath.IsAbs(p) || path.IsAbs(slash) {
		return "", fmt.Errorf("path must be target-relative")
	}
	if slices.Contains(strings.Split(slash, "/"), "..") {
		return "", fmt.Errorf("path must not contain '..'")
	}
	return path.Clean(slash), nil
}

// ValidateFile parses the file at path and reports schema problems as an
// error. A missing file is an error. Unlike Config.Validate, this also requires
// Target to be set, since the only way to fix it from a file is to edit the
// file.
func ValidateFile(filename string) error {
	c, _, err := LoadRequired(filename)
	if err != nil {
		return err
	}
	if c.Target == "" {
		return fmt.Errorf("%s: 'target' is required", filename)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("%s: %w", filename, err)
	}
	return nil
}
