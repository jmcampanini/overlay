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
)

const (
	configPathSources     = "sources"
	configPathTarget      = "target"
	configPathProfiles    = "profiles"
	configPathIgnore      = "ignore"
	configPathRenderRules = "render_rules"
)

// Resolved is the runtime settings bundle after raw config loading and
// Overlay-specific derivation/validation.
type Resolved struct {
	Settings        discover.Settings
	ContinueOnError bool
	Logger          *log.Logger
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
	derivationErrors []effectiveConfigError
}

// resolve merges config file, environment variables, and CLI flags into a
// runtime settings bundle. It returns an error when Overlay-specific runtime
// validation fails.
func resolve(command *cobra.Command, flags *globalFlags, positionalSources ...string) (Resolved, error) {
	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})
	switch {
	case flags.verbose:
		logger.SetLevel(log.DebugLevel)
	case flags.quiet:
		logger.SetLevel(log.WarnLevel)
	default:
		logger.SetLevel(log.InfoLevel)
	}

	r := Resolved{
		Logger: logger,
	}

	raw, err := loadRawConfig(command, flags)
	if err != nil {
		return r, err
	}
	r.RawConfig = raw.Config

	effective := deriveEffectiveConfig(raw, positionalSources...)
	r.Effective = effective
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

func loadRawConfig(command *cobra.Command, flags *globalFlags) (rawLoadedConfig, error) {
	cfgPath := flags.config
	if cfgPath == "" {
		cfgPath = config.DefaultFilename
	}
	configExplicit := changed(command, "config")
	return loadRawConfigFromPath(command, cfgPath, configExplicit)
}

func loadRawConfigFromPath(command *cobra.Command, cfgPath string, required bool) (rawLoadedConfig, error) {
	fileLoader, err := config.NewFileLoader(cfgPath, required)
	if err != nil {
		return rawLoadedConfig{}, err
	}
	envLoader, err := configloader.NewEnvironmentLoader[config.Config]("overlay", configloader.OSEnv())
	if err != nil {
		return rawLoadedConfig{}, err
	}
	flagLoader, err := pflagloader.NewLoader[config.Config](command.Flags())
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
	target, targetErrors := derivePath(configPathTarget, cfg.Target, raw.Report, configBase, configExists)
	profiles := effectiveProfiles(cfg)
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
	}
}

func deriveSourceDirs(positional []string, cfg config.Config, report configloader.LoadReport, configBase string, configExists bool) sourceResolution {
	if len(positional) > 0 {
		return deriveSourceValues(positional, configExists, configBase)
	}

	sourcesSource := report.Updates[configPathSources]
	anchor := sourceIsFile(sourcesSource) || (sourcesSource == configloader.SourceDefault && configExists)
	return deriveSourceValues(cfg.Sources, anchor, configBase)
}

func deriveSourceValues(values []string, anchor bool, configBase string) sourceResolution {
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
		derivationErrors: derivationErrors,
	}
}

func derivePath(name, value string, report configloader.LoadReport, configBase string, configExists bool) (string, []effectiveConfigError) {
	source := report.Updates[name]
	p := value
	if sourceIsFile(source) || (source == configloader.SourceDefault && configExists) {
		p = resolveRelative(value, configBase)
	}
	expanded, err := discover.ExpandPath(p)
	if err != nil {
		return p, []effectiveConfigError{{Field: name, Err: fmt.Errorf("expand %s: %w", name, err)}}
	}
	return expanded, nil
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

func effectiveProfiles(cfg config.Config) []string {
	out := append([]string{}, cfg.Profiles...)
	if cfg.EnvProfiles == "" {
		return dedupe(out)
	}
	extra := splitCSV(os.Getenv(cfg.EnvProfiles))
	out = append(out, extra...)
	return dedupe(out)
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
