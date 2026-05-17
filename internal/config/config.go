// Package config loads and validates Overlay raw configuration.
package config

import (
	"fmt"

	configloader "github.com/jmcampanini/go-config-loader"
)

// DefaultFilename is the conventional location for the overlay config.
const DefaultFilename = ".overlay.toml"

// Config mirrors the .overlay.toml schema. Fields tagged with config are
// loadable from environment variables and pflags; TOML-only fields are not.
type Config struct {
	Sources          []string `toml:"sources" config:"sources" pflag_singular:"source" help:"override source directories from config (repeatable or comma-separated); --source adds one source"`
	Target           string   `toml:"target" config:"target" help:"override target directory from config"`
	DotPrefix        bool     `toml:"dot_prefix"`
	Profiles         []string `toml:"profiles" config:"profiles" help:"comma-separated profile list"`
	EnvProfiles      string   `toml:"env_profiles"`
	ContinueOnError  bool     `toml:"continue_on_error" config:"continue" help:"continue past invalid source files"`
	Ignore           []string `toml:"ignore"`
	TraverseHidden   bool     `toml:"traverse_hidden"`
	RespectGitignore bool     `toml:"respect_gitignore"`
}

// fileConfig is a TOML-only compatibility shim for legacy source = "dir".
type fileConfig struct {
	Source           string   `toml:"source"`
	Sources          []string `toml:"sources"`
	Target           string   `toml:"target"`
	DotPrefix        bool     `toml:"dot_prefix"`
	Profiles         []string `toml:"profiles"`
	EnvProfiles      string   `toml:"env_profiles"`
	ContinueOnError  bool     `toml:"continue_on_error"`
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
	loader, err := newFileConfigLoader(path, required)
	if err != nil {
		return nil, err
	}
	return func(base Config) (Config, configloader.LoadReport, error) {
		loaded, report, err := loader(fileConfigFromConfig(base))
		if err != nil {
			return base, configloader.LoadReport{}, err
		}
		if err := validateFileSourceSelection(report); err != nil {
			return base, configloader.LoadReport{}, err
		}
		cfg := loaded.toConfig()
		if source, ok := fileUpdate(report.Updates["source"]); ok {
			cfg.Sources = []string{loaded.Source}
			report.Updates["sources"] = source
		}
		delete(report.Updates, "source")
		return cfg, report, nil
	}, nil
}

func newFileConfigLoader(path string, required bool) (configloader.ConfigLoader[fileConfig], error) {
	if required {
		return configloader.NewRequiredFileLoader[fileConfig](path)
	}
	return configloader.NewMergeAllFilesLoader[fileConfig](configloader.File(path))
}

func fileConfigFromConfig(c Config) fileConfig {
	return fileConfig{
		Sources:          cloneStrings(c.Sources),
		Target:           c.Target,
		DotPrefix:        c.DotPrefix,
		Profiles:         cloneStrings(c.Profiles),
		EnvProfiles:      c.EnvProfiles,
		ContinueOnError:  c.ContinueOnError,
		Ignore:           cloneStrings(c.Ignore),
		TraverseHidden:   c.TraverseHidden,
		RespectGitignore: c.RespectGitignore,
	}
}

func (c fileConfig) toConfig() Config {
	return Config{
		Sources:          cloneStrings(c.Sources),
		Target:           c.Target,
		DotPrefix:        c.DotPrefix,
		Profiles:         cloneStrings(c.Profiles),
		EnvProfiles:      c.EnvProfiles,
		ContinueOnError:  c.ContinueOnError,
		Ignore:           cloneStrings(c.Ignore),
		TraverseHidden:   c.TraverseHidden,
		RespectGitignore: c.RespectGitignore,
	}
}

func cloneStrings(xs []string) []string {
	if xs == nil {
		return nil
	}
	return append([]string{}, xs...)
}

func validateFileSourceSelection(report configloader.LoadReport) error {
	sourceFile, hasSource := fileUpdate(report.Updates["source"])
	sourcesFile, hasSources := fileUpdate(report.Updates["sources"])
	if hasSource && hasSources {
		if sourceFile == sourcesFile {
			return fmt.Errorf("%s: set either 'source' or 'sources', not both", sourceFile)
		}
		return fmt.Errorf("set either 'source' (%s) or 'sources' (%s), not both", sourceFile, sourcesFile)
	}
	return nil
}

func fileUpdate(source string) (string, bool) {
	switch source {
	case "", configloader.SourceDefault:
		return "", false
	default:
		return source, true
	}
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
