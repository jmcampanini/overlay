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

type configEffective struct {
	SourceDirs       []string
	SourceLabels     []string
	TargetDir        string
	Profiles         []string
	ContinueOnError  bool
	Provenance       Provenances
	DerivationErrors []configEffectiveError
}

type configEffectiveError struct {
	Field string
	Err   error
}

type sourceResolution struct {
	dirs       []string
	labels     []string
	provenance Provenance
	errors     []configEffectiveError
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

	effective := deriveConfigEffective(raw, positionalSources...)
	r.Provenance = effective.Provenance
	r.SourceLabels = effective.SourceLabels
	if err := firstConfigEffectiveError(validateConfigEffective(raw, effective)); err != nil {
		return r, err
	}
	r.ContinueOnError = effective.ContinueOnError

	globIgn, err := discover.NewGlobIgnorer(raw.Config.Ignore)
	if err != nil {
		return r, err
	}

	r.Settings = discover.Settings{
		SourceDirs:       effective.SourceDirs,
		TargetDir:        effective.TargetDir,
		DotPrefix:        raw.Config.DotPrefix,
		Profiles:         effective.Profiles,
		Ignore:           globIgn,
		TraverseHidden:   raw.Config.TraverseHidden,
		RespectGitignore: raw.Config.RespectGitignore,
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

func configBaseFromReport(report configloader.LoadReport) (string, bool) {
	if len(report.LoadedFiles) == 0 {
		return ".", false
	}
	return filepath.Dir(report.LoadedFiles[0]), true
}

func deriveConfigEffective(raw rawLoadedConfig, positionalSources ...string) configEffective {
	configBase, configExists := configBaseFromReport(raw.Report)
	cfg := raw.Config

	sources := deriveSourceDirs(positionalSources, cfg, raw.Report, configBase, configExists)
	target, targetProv, targetErrors := derivePath(configPathTarget, cfg.Target, raw.Report, configBase, configExists)
	profiles, envContributed := effectiveProfiles(cfg)

	effective := configEffective{
		SourceDirs:      sources.dirs,
		SourceLabels:    sources.labels,
		TargetDir:       target,
		Profiles:        profiles,
		ContinueOnError: cfg.ContinueOnError,
		Provenance: Provenances{
			Source:          sources.provenance,
			Target:          targetProv,
			Profiles:        profilesProvenance(raw.Report, envContributed),
			ContinueOnError: provenanceFromReport(raw.Report, configPathContinueOnError),
		},
	}
	effective.DerivationErrors = append(effective.DerivationErrors, sources.errors...)
	effective.DerivationErrors = append(effective.DerivationErrors, targetErrors...)
	return effective
}

func deriveSourceDirs(positional []string, cfg config.Config, report configloader.LoadReport, configBase string, configExists bool) sourceResolution {
	if len(positional) > 0 {
		values := append([]string(nil), positional...)
		return deriveSourceValues(configPathSources, values, values, ProvFlag, configExists, configBase)
	}

	sourcesSource := report.Updates[configPathSources]
	values := append([]string(nil), cfg.Sources...)
	anchor := sourceIsFile(sourcesSource) || (sourcesSource == configloader.SourceDefault && configExists)
	return deriveSourceValues(configPathSources, values, values, provenanceFromSource(sourcesSource), anchor, configBase)
}

func deriveSourceValues(name string, values, labels []string, prov Provenance, anchor bool, configBase string) sourceResolution {
	dirs := make([]string, 0, len(values))
	var errors []configEffectiveError
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			dirs = append(dirs, value)
			continue
		}
		p := value
		if anchor {
			p = resolveRelative(p, configBase)
		}
		expanded, err := discover.ExpandPath(p)
		if err != nil {
			errors = append(errors, configEffectiveError{Field: name, Err: fmt.Errorf("expand %s: %w", name, err)})
			dirs = append(dirs, p)
			continue
		}
		dirs = append(dirs, expanded)
	}

	return sourceResolution{
		dirs:       dirs,
		labels:     append([]string(nil), labels...),
		provenance: prov,
		errors:     errors,
	}
}

func derivePath(name, value string, report configloader.LoadReport, configBase string, configExists bool) (string, Provenance, []configEffectiveError) {
	source := report.Updates[name]
	p := value
	if sourceIsFile(source) || (source == configloader.SourceDefault && configExists) {
		p = resolveRelative(value, configBase)
	}
	prov := provenanceFromSource(source)
	expanded, err := discover.ExpandPath(p)
	if err != nil {
		return p, prov, []configEffectiveError{{Field: name, Err: fmt.Errorf("expand %s: %w", name, err)}}
	}
	return expanded, prov, nil
}

func validateConfigEffective(raw rawLoadedConfig, effective configEffective) []configEffectiveError {
	errors := append([]configEffectiveError(nil), effective.DerivationErrors...)
	if len(effective.SourceDirs) == 0 {
		errors = append(errors, configEffectiveError{Field: configPathSources, Err: fmt.Errorf("%s is empty", configPathSources)})
	}
	for _, source := range effective.SourceDirs {
		if strings.TrimSpace(source) == "" {
			errors = append(errors, configEffectiveError{Field: configPathSources, Err: fmt.Errorf("%s contains an empty source directory", configPathSources)})
			break
		}
	}
	if effective.TargetDir == "" {
		errors = append(errors, configEffectiveError{Field: configPathTarget, Err: fmt.Errorf("target is required (set in %s or pass --target)", raw.ConfigPath)})
	}
	if err := config.ValidateProfiles(effective.Profiles); err != nil {
		errors = append(errors, configEffectiveError{Field: configPathProfiles, Err: err})
	}
	return errors
}

func firstConfigEffectiveError(errors []configEffectiveError) error {
	if len(errors) == 0 {
		return nil
	}
	return errors[0].Err
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

func sourceIsFile(source string) bool {
	switch source {
	case "", configloader.SourceDefault, configloader.SourceEnv, pflagloader.SourcePFlag:
		return false
	default:
		return true
	}
}
