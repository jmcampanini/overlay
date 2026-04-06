// Package config loads and validates the .overlay.toml file.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// DefaultFilename is the conventional location for the overlay config.
const DefaultFilename = ".overlay.toml"

// Config mirrors the .overlay.toml schema. All fields use their natural
// type; starting from Default() and overlaying a parsed file gives the
// correct result because pelletier/go-toml only touches keys present in
// the file.
type Config struct {
	Source           string   `toml:"source"`
	Target           string   `toml:"target"`
	DotPrefix        bool     `toml:"dot_prefix"`
	Profiles         []string `toml:"profiles"`
	EnvProfiles      string   `toml:"env_profiles"`
	ContinueOnError  bool     `toml:"continue_on_error"`
	Ignore           []string `toml:"ignore"`
	TraverseHidden   bool     `toml:"traverse_hidden"`
	RespectGitignore bool     `toml:"respect_gitignore"`
}

// Default returns a Config populated with the default values. Callers
// should start from Default() and then overlay user settings on top.
func Default() Config {
	return Config{
		Source:    ".",
		DotPrefix: true,
		Profiles:  []string{},
		Ignore:    []string{},
	}
}

// Load reads and parses a .overlay.toml file. The returned bool reports
// whether the file existed; a missing file yields Default() and no error.
// When the file is present, Load starts from Default() and overlays the
// file's settings so any field the user omits keeps its default.
// Unknown keys are rejected so typos fail fast on every command path.
func Load(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), false, nil
		}
		return Config{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	c := Default()
	if err := decodeStrict(path, data, &c); err != nil {
		return Config{}, true, err
	}
	return c, true, nil
}

// decodeStrict decodes TOML into c with unknown fields rejected, returning
// an error formatted with path context. Callers own file reading and the
// "missing file" policy.
func decodeStrict(path string, data []byte, c *Config) error {
	dec := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		var strictErr *toml.StrictMissingError
		if errors.As(err, &strictErr) {
			return fmt.Errorf("unknown fields in %s:\n%s", path, strictErr.String())
		}
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// LoadKeys returns the set of top-level keys explicitly present in the
// given file. It is used by the CLI resolver to report accurate "from:"
// provenance for every field, including booleans that may match the
// default value. A missing file yields an empty set and no error.
func LoadKeys(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var raw map[string]any
	if err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, nil
}
