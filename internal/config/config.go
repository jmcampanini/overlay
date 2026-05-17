// Package config loads and validates Overlay raw configuration.
package config

import (
	configloader "github.com/jmcampanini/go-config-loader"
)

// DefaultFilename is the conventional location for the overlay config.
const DefaultFilename = ".overlay.toml"

// Config mirrors the .overlay.toml schema. Fields tagged with config are
// loadable from environment variables and pflags; TOML-only fields are not.
type Config struct {
	Sources          []string `toml:"sources" config:"sources" pflag_singular:"source" help:"override source directories from config"`
	Target           string   `toml:"target" config:"target" help:"override target directory from config"`
	DotPrefix        bool     `toml:"dot_prefix"`
	Profiles         []string `toml:"profiles" config:"profiles" help:"comma-separated profile list"`
	EnvProfiles      string   `toml:"env_profiles"`
	ContinueOnError  bool     `toml:"continue_on_error" config:"continue" help:"continue past invalid source files"`
	Ignore           []string `toml:"ignore"`
	TraverseHidden   bool     `toml:"traverse_hidden"`
	RespectGitignore bool     `toml:"respect_gitignore"`
}

// Default returns a Config populated with the default raw values.
func Default() Config {
	return Config{
		Sources:   []string{"."},
		DotPrefix: true,
		Profiles:  []string{},
		Ignore:    []string{},
	}
}

// NewFileLoader returns a GoConfigLoader file loader for path. When required is
// true, path must exist; otherwise a missing file leaves defaults unchanged.
func NewFileLoader(path string, required bool) (configloader.ConfigLoader[Config], error) {
	if required {
		return configloader.NewRequiredFileLoader[Config](path)
	}
	return configloader.NewMergeAllFilesLoader[Config](configloader.File(path))
}

// Load reads an optional raw .overlay.toml file. The returned bool reports
// whether the file existed; a missing file yields Default() and no error.
func Load(path string) (Config, bool, configloader.LoadReport, error) {
	cfg, report, err := load(path, false)
	if err != nil {
		return Config{}, false, configloader.LoadReport{}, err
	}
	return cfg, len(report.LoadedFiles) > 0, report, nil
}

// LoadRequired reads a required raw .overlay.toml file.
func LoadRequired(path string) (Config, configloader.LoadReport, error) {
	return load(path, true)
}

func load(path string, required bool) (Config, configloader.LoadReport, error) {
	loader, err := NewFileLoader(path, required)
	if err != nil {
		return Config{}, configloader.LoadReport{}, err
	}
	cfg, report, err := configloader.Load(Default(), loader)
	if err != nil {
		return Config{}, configloader.LoadReport{}, err
	}
	return cfg, report, nil
}
