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

// Validate parses the file at path and reports schema problems as an error.
// On success it returns nil. A missing file is an error (use Load for the
// "missing is OK" flow).
func Validate(path string) error {
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
	for _, p := range c.Profiles {
		if slices.Contains(reservedProfiles, p) {
			return fmt.Errorf("%s: profile name %q is reserved (base and local are special layers)", path, p)
		}
	}
	return nil
}
