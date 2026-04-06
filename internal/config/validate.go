package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
)

// reservedProfiles cannot appear in the profiles list — they name the
// special base and local layers.
var reservedProfiles = []string{"base", "local"}

// Validate checks the in-memory Config for schema-level problems.
// Reserved profile names are rejected here. Target is NOT required by
// this method because flag/env resolution can supply it later; the CLI
// resolver checks the final value.
func (c Config) Validate() error {
	for _, p := range c.Profiles {
		if slices.Contains(reservedProfiles, p) {
			return fmt.Errorf("profile name %q is reserved (base and local are special layers)", p)
		}
	}
	return nil
}

// ValidateFile parses the file at path and reports schema problems as
// an error. On success it returns nil. A missing file is an error (use
// Load for the "missing is OK" flow). Unlike Config.Validate, this also
// requires Target to be set, since the only way to fix it from a file
// is to edit the file.
func ValidateFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("config file %s does not exist", path)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := decodeStrict(path, data, &c); err != nil {
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
