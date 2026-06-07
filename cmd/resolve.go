package cmd

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
	configPathIgnore          = "ignore"
	configPathRenderRules     = "render_rules"
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
	Effective       effectiveConfig
	SourceLabels    []string
}

type rawLoadedConfig struct {
	Config     config.Config
	Report     configloader.LoadReport
	ConfigPath string
}

type effectiveConfig struct {
	SourceDirs       []string
	SourceLabels     []string
	TargetDir        string
	DotPrefix        bool
	Profiles         []string
	ContinueOnError  bool
	TOMLIndentTables bool
	Ignore           []string
	TraverseHidden   bool
	RespectGitignore bool
	RenderRules      []config.RenderRule
	Provenance       Provenances
	DerivationErrors []effectiveConfigError
}

type effectiveConfigError struct {
	Field string
	Err   error
}

type effectiveConfigErrors []effectiveConfigError

func (errs effectiveConfigErrors) Err() error {
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func (errs effectiveConfigErrors) FirstError() error {
	if len(errs) == 0 {
		return nil
	}
	return errs[0].Err
}

func (errs effectiveConfigErrors) Error() string {
	lines := make([]string, len(errs))
	for i, effectiveErr := range errs {
		lines[i] = fmt.Sprintf("%s: %v", effectiveErr.Field, effectiveErr.Err)
	}
	return strings.Join(lines, "\n")
}

type sourceResolution struct {
	dirs             []string
	labels           []string
	provenance       Provenance
	derivationErrors []effectiveConfigError
}

// resolve merges config file, environment variables, and CLI flags into a
// runtime settings bundle. It returns an error when Overlay-specific runtime
// validation fails.
func resolve(cmd *cobra.Command, g *globalFlags, positionalSources ...string) (Resolved, error) {
	r := Resolved{
		Logger: logging.Setup(g.quiet, g.verbose),
	}

	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		return r, err
	}
	r.RawConfig = raw.Config

	effective := deriveEffectiveConfig(raw, positionalSources...)
	r.Effective = effective
	r.Provenance = effective.Provenance
	r.SourceLabels = effective.SourceLabels
	effectiveErrors := effectiveConfigErrors(validateEffectiveConfig(raw, effective))
	if err := effectiveErrors.FirstError(); err != nil {
		return r, err
	}
	r.ContinueOnError = effective.ContinueOnError

	globIgn, err := discover.NewGlobIgnorer(effective.Ignore)
	if err != nil {
		return r, err
	}

	r.Settings = discover.Settings{
		SourceDirs:       effective.SourceDirs,
		TargetDir:        effective.TargetDir,
		DotPrefix:        effective.DotPrefix,
		Profiles:         effective.Profiles,
		Ignore:           globIgn,
		TraverseHidden:   effective.TraverseHidden,
		RespectGitignore: effective.RespectGitignore,
	}
	return r, nil
}

func loadRawConfig(cmd *cobra.Command, g *globalFlags) (rawLoadedConfig, error) {
	cfgPath := g.config
	configExplicit := changed(cmd, "config")
	if cfgPath == "" {
		cfgPath = config.DefaultFilename
	}
	return loadRawConfigFromPath(cmd, cfgPath, configExplicit)
}

func loadRawConfigFromPath(cmd *cobra.Command, cfgPath string, required bool) (rawLoadedConfig, error) {
	fileLoader, err := config.NewFileLoader(cfgPath, required)
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

func deriveEffectiveConfig(raw rawLoadedConfig, positionalSources ...string) effectiveConfig {
	configBase, configExists := configBaseFromReport(raw.Report)
	cfg := raw.Config

	sources := deriveSourceDirs(positionalSources, cfg, raw.Report, configBase, configExists)
	target, targetProv, targetErrors := derivePath(configPathTarget, cfg.Target, raw.Report, configBase, configExists)
	profiles, envContributed := effectiveProfiles(cfg)
	ignore, ignoreErrors := deriveIgnorePatterns(cfg.Ignore)
	renderRules, renderRuleErrors := deriveRenderRules(cfg.RenderRules)

	derivationErrors := append([]effectiveConfigError(nil), sources.derivationErrors...)
	derivationErrors = append(derivationErrors, targetErrors...)
	derivationErrors = append(derivationErrors, ignoreErrors...)
	derivationErrors = append(derivationErrors, renderRuleErrors...)

	return effectiveConfig{
		SourceDirs:       sources.dirs,
		SourceLabels:     sources.labels,
		TargetDir:        target,
		DotPrefix:        cfg.DotPrefix,
		Profiles:         profiles,
		ContinueOnError:  cfg.ContinueOnError,
		TOMLIndentTables: cfg.TOMLIndentTables,
		Ignore:           ignore,
		TraverseHidden:   cfg.TraverseHidden,
		RespectGitignore: cfg.RespectGitignore,
		RenderRules:      renderRules,
		DerivationErrors: derivationErrors,
		Provenance: Provenances{
			Source:          sources.provenance,
			Target:          targetProv,
			Profiles:        profilesProvenance(raw.Report, envContributed),
			ContinueOnError: provenanceFromReport(raw.Report, configPathContinueOnError),
		},
	}
}

func deriveSourceDirs(positional []string, cfg config.Config, report configloader.LoadReport, configBase string, configExists bool) sourceResolution {
	if len(positional) > 0 {
		return deriveSourceValues(positional, ProvFlag, configExists, configBase)
	}

	sourcesSource := report.Updates[configPathSources]
	anchor := sourceIsFile(sourcesSource) || (sourcesSource == configloader.SourceDefault && configExists)
	return deriveSourceValues(cfg.Sources, provenanceFromSource(sourcesSource), anchor, configBase)
}

func deriveSourceValues(values []string, prov Provenance, anchor bool, configBase string) sourceResolution {
	dirs := make([]string, 0, len(values))
	var derivationErrors []effectiveConfigError
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
			derivationErrors = append(derivationErrors, effectiveConfigError{Field: configPathSources, Err: fmt.Errorf("expand %s: %w", configPathSources, err)})
			dirs = append(dirs, p)
			continue
		}
		dirs = append(dirs, expanded)
	}

	return sourceResolution{
		dirs:             dirs,
		labels:           append([]string(nil), values...),
		provenance:       prov,
		derivationErrors: derivationErrors,
	}
}

func derivePath(name, value string, report configloader.LoadReport, configBase string, configExists bool) (string, Provenance, []effectiveConfigError) {
	source := report.Updates[name]
	p := value
	if sourceIsFile(source) || (source == configloader.SourceDefault && configExists) {
		p = resolveRelative(value, configBase)
	}
	prov := provenanceFromSource(source)
	expanded, err := discover.ExpandPath(p)
	if err != nil {
		return p, prov, []effectiveConfigError{{Field: name, Err: fmt.Errorf("expand %s: %w", name, err)}}
	}
	return expanded, prov, nil
}

func deriveIgnorePatterns(patterns []string) ([]string, []effectiveConfigError) {
	normalized, err := discover.NormalizeGlobPatterns(patterns)
	if err != nil {
		return append([]string(nil), patterns...), []effectiveConfigError{{Field: configPathIgnore, Err: err}}
	}
	return normalized, nil
}

func deriveRenderRules(rules []config.RenderRule) ([]config.RenderRule, []effectiveConfigError) {
	normalized, err := config.NormalizeRenderRules(rules)
	if err != nil {
		return append([]config.RenderRule(nil), rules...), []effectiveConfigError{{Field: configPathRenderRules, Err: err}}
	}
	return normalized, nil
}

func validateEffectiveConfig(raw rawLoadedConfig, effective effectiveConfig) []effectiveConfigError {
	errors := append([]effectiveConfigError(nil), effective.DerivationErrors...)
	if len(effective.SourceDirs) == 0 {
		errors = append(errors, effectiveConfigError{Field: configPathSources, Err: fmt.Errorf("%s is empty", configPathSources)})
	}
	for _, source := range effective.SourceDirs {
		if strings.TrimSpace(source) == "" {
			errors = append(errors, effectiveConfigError{Field: configPathSources, Err: fmt.Errorf("%s contains an empty source directory", configPathSources)})
			break
		}
	}
	if effective.TargetDir == "" {
		errors = append(errors, effectiveConfigError{Field: configPathTarget, Err: fmt.Errorf("target is required (set in %s or pass --target)", raw.ConfigPath)})
	}
	if err := config.ValidateProfiles(effective.Profiles); err != nil {
		errors = append(errors, effectiveConfigError{Field: configPathProfiles, Err: err})
	}
	return errors
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
