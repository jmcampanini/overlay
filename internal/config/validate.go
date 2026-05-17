package config

import (
	"fmt"
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
	return ValidateProfiles(c.Profiles)
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
