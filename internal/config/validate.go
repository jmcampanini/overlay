package config

import (
	"fmt"
	"slices"
)

// reservedProfiles cannot appear in the profiles list — they name the
// special base and local layers.
var reservedProfiles = []string{"base", "local"}

// Validate checks the in-memory Config for Overlay-specific schema problems.
// Target is not required by this method because env/flag/runtime resolution can
// supply it later; the CLI resolver checks the final value.
func (c Config) Validate() error {
	for _, p := range c.Profiles {
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
