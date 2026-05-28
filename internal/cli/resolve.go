// Package cli wires the cobra commands for the overlay binary and
// resolves config + env + flags into the Settings consumed by the
// render, diff, and plan packages.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/log/v2"
	"github.com/jmcampanini/go-config-loader/configloader"
	"github.com/jmcampanini/go-config-loader/pflagloader"
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/logging"
)

const (
	configPathSources         = "sources"
	configPathTarget          = "target"
	configPathProfiles        = "profiles"
	configPathContinueOnError = "continueonerror"
)

// Provenance identifies which raw source produced a setting's value.
type Provenance int

// Provenance constants. The integer values are not compared anywhere;
// the constants exist solely as discriminated tags.
const (
	ProvDefault   Provenance = iota // built-in default
	ProvEnv                         // environment variable
	ProvConfig                      // .overlay.toml
	ProvConfigEnv                   // raw profiles plus env_profiles contribution
	ProvFlag                        // CLI flag override
)

// String returns a display label for the provenance.
func (p Provenance) String() string {
	switch p {
	case ProvFlag:
		return "flag"
	case ProvConfigEnv:
		return "config+env"
	case ProvConfig:
		return "config"
	case ProvEnv:
		return "env"
	}
	return "default"
}

// Provenances records where each runtime setting's raw value came from.
type Provenances struct {
	Source          Provenance
	Target          Provenance
	Profiles        Provenance
	ContinueOnError Provenance
}

// Resolved is the runtime settings bundle after raw config loading and
// Overlay-specific derivation/validation.
type Resolved struct {
	Settings        discover.Settings
	ContinueOnError bool
	Logger          *log.Logger
	Provenance      Provenances
	RawConfig       config.Config
	SourceLabels    []string
}

type rawLoadedConfig struct {
	Config     config.Config
	Report     configloader.LoadReport
	ConfigPath string
}

type sourceResolution struct {
	dirs       []string
	labels     []string
	provenance Provenance
}

// Resolve merges config file, environment variables, and CLI flags into a
// runtime settings bundle. It returns an error when Overlay-specific runtime
// validation fails.
func Resolve(cmd *cobra.Command, g *GlobalFlags, positionalSources ...string) (Resolved, error) {
	r := Resolved{
		Logger: logging.Setup(g.Quiet, g.Verbose),
	}

	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		return r, err
	}
	r.RawConfig = raw.Config

	configExists := len(raw.Report.LoadedFiles) > 0
	configBase := "."
	if configExists {
		configBase = filepath.Dir(raw.Report.LoadedFiles[0])
	}

	cfg := raw.Config
	sources, err := resolveSourceDirs(positionalSources, cfg, raw.Report, configBase, configExists)
	if err != nil {
		return r, err
	}
	r.Provenance.Source = sources.provenance
	r.SourceLabels = sources.labels

	target, targetProv, err := resolvePath(configPathTarget, cfg.Target, raw.Report, configBase, configExists)
	if err != nil {
		return r, err
	}
	r.Provenance.Target = targetProv
	if target == "" {
		return r, fmt.Errorf("target is required (set in %s or pass --target)", raw.ConfigPath)
	}

	r.Provenance.ContinueOnError = provenanceFromReport(raw.Report, configPathContinueOnError)

	profiles, envContributed := effectiveProfiles(cfg)
	r.Provenance.Profiles = profilesProvenance(raw.Report, envContributed)
	if err := config.ValidateProfiles(profiles); err != nil {
		return r, err
	}

	r.ContinueOnError = cfg.ContinueOnError

	globIgn, err := discover.NewGlobIgnorer(cfg.Ignore)
	if err != nil {
		return r, err
	}

	r.Settings = discover.Settings{
		SourceDirs:       sources.dirs,
		TargetDir:        target,
		DotPrefix:        cfg.DotPrefix,
		Profiles:         profiles,
		Ignore:           globIgn,
		TraverseHidden:   cfg.TraverseHidden,
		RespectGitignore: cfg.RespectGitignore,
	}
	return r, nil
}

func loadRawConfig(cmd *cobra.Command, g *GlobalFlags) (rawLoadedConfig, error) {
	cfgPath := g.Config
	configExplicit := changed(cmd, "config")
	if cfgPath == "" {
		cfgPath = config.DefaultFilename
	}

	fileLoader, err := config.NewFileLoader(cfgPath, configExplicit)
	if err != nil {
		return rawLoadedConfig{}, err
	}
	envLoader, err := configloader.NewEnvironmentLoader[config.Config]("overlay", configloader.OSEnv())
	if err != nil {
		return rawLoadedConfig{}, err
	}
	flagLoader, err := pflagloader.NewLoader[config.Config](cmd.Flags())
	if err != nil {
		return rawLoadedConfig{}, err
	}

	cfg, report, err := configloader.Load(config.Default(), fileLoader, envLoader, flagLoader)
	if err != nil {
		return rawLoadedConfig{}, err
	}

	return rawLoadedConfig{
		Config:     cfg,
		Report:     report,
		ConfigPath: cfgPath,
	}, nil
}

func resolveSourceDirs(positional []string, cfg config.Config, report configloader.LoadReport, configBase string, configExists bool) (sourceResolution, error) {
	if len(positional) > 0 {
		values := append([]string(nil), positional...)
		return resolveSourceValues(configPathSources, values, values, ProvFlag, configExists, configBase)
	}

	sourcesSource := report.Updates[configPathSources]
	values := append([]string(nil), cfg.Sources...)
	anchor := sourceIsFile(sourcesSource) || (sourcesSource == configloader.SourceDefault && configExists)
	return resolveSourceValues(configPathSources, values, values, provenanceFromSource(sourcesSource), anchor, configBase)
}

func resolveSourceValues(name string, values, labels []string, prov Provenance, anchor bool, configBase string) (sourceResolution, error) {
	if len(values) == 0 {
		return sourceResolution{}, fmt.Errorf("%s is empty", name)
	}

	dirs := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return sourceResolution{}, fmt.Errorf("%s contains an empty source directory", name)
		}
		p := value
		if anchor {
			p = resolveRelative(p, configBase)
		}
		expanded, err := discover.ExpandPath(p)
		if err != nil {
			return sourceResolution{}, fmt.Errorf("expand %s: %w", name, err)
		}
		dirs = append(dirs, expanded)
	}

	return sourceResolution{
		dirs:       dirs,
		labels:     append([]string(nil), labels...),
		provenance: prov,
	}, nil
}

func effectiveProfiles(cfg config.Config) ([]string, bool) {
	out := append([]string{}, cfg.Profiles...)
	if cfg.EnvProfiles == "" {
		return dedupe(out), false
	}
	extra := splitCSV(os.Getenv(cfg.EnvProfiles))
	out = append(out, extra...)
	return dedupe(out), len(extra) > 0
}

func profilesProvenance(report configloader.LoadReport, envContributed bool) Provenance {
	if envContributed {
		return ProvConfigEnv
	}
	return provenanceFromReport(report, configPathProfiles)
}

func provenanceFromReport(report configloader.LoadReport, path string) Provenance {
	return provenanceFromSource(report.Updates[path])
}

func provenanceFromSource(source string) Provenance {
	switch source {
	case pflagloader.SourcePFlag:
		return ProvFlag
	case configloader.SourceEnv:
		return ProvEnv
	case "", configloader.SourceDefault:
		return ProvDefault
	default:
		return ProvConfig
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func dedupe(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// resolveRelative turns a relative path from the config file into one anchored
// at the config file's directory. Absolute paths and paths starting with ~ or
// $VAR are left alone for ExpandPath to handle.
func resolveRelative(p, base string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if strings.HasPrefix(p, "~") || strings.Contains(p, "$") {
		return p
	}
	return filepath.Join(base, p)
}

func resolvePath(name, value string, report configloader.LoadReport, configBase string, configExists bool) (string, Provenance, error) {
	source := report.Updates[name]
	p := value
	if sourceIsFile(source) || (source == configloader.SourceDefault && configExists) {
		p = resolveRelative(value, configBase)
	}
	prov := provenanceFromSource(source)
	expanded, err := discover.ExpandPath(p)
	if err != nil {
		return "", prov, fmt.Errorf("expand %s: %w", name, err)
	}
	return expanded, prov, nil
}

func sourceIsFile(source string) bool {
	switch source {
	case "", configloader.SourceDefault, configloader.SourceEnv, pflagloader.SourcePFlag:
		return false
	default:
		return true
	}
}
