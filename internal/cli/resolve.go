// Package cli wires the cobra commands for the overlay binary and
// resolves config + env + flags into the Settings consumed by the
// render, diff, and plan packages.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/logging"
)

// DefaultEnvProfiles is the fallback env var name used when no config
// file is found and no --profiles flag is given.
const DefaultEnvProfiles = "OVERLAY_PROFILES"

// Provenance identifies which resolution path produced a setting's value.
type Provenance int

// Provenance constants in order of increasing specificity. The String()
// method returns the labels printed by `overlay config` ("default",
// "env", "config", "config+env", "flag").
const (
	ProvDefault   Provenance = iota // built-in default
	ProvEnv                         // OVERLAY_PROFILES env var
	ProvConfig                      // .overlay.toml only
	ProvConfigEnv                   // .overlay.toml + env_profiles env var
	ProvFlag                        // CLI flag override
)

// String returns the label printed by `overlay config`.
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

// ConfigKey is a typed name for a top-level field in .overlay.toml. Each
// constant matches the TOML key name exactly so it can be used as both
// the Provenance map key and the printed label.
type ConfigKey string

// Recognized configuration keys.
const (
	KeySource           ConfigKey = "source"
	KeyTarget           ConfigKey = "target"
	KeyDotPrefix        ConfigKey = "dot_prefix"
	KeyProfiles         ConfigKey = "profiles"
	KeyContinueOnError  ConfigKey = "continue_on_error"
	KeyTraverseHidden   ConfigKey = "traverse_hidden"
	KeyRespectGitignore ConfigKey = "respect_gitignore"
	KeyIgnore           ConfigKey = "ignore"
)

// Resolved is the fully-resolved set of settings after applying the
// config-file + env + flag precedence rules. Provenance records where
// each setting's value came from so `overlay config` can annotate output.
type Resolved struct {
	Settings        discover.Settings
	ContinueOnError bool
	Logger          *log.Logger
	ConfigPath      string
	ConfigExists    bool
	Provenance      map[ConfigKey]Provenance
}

// Resolve merges config file, env vars, and CLI flags into a Resolved
// settings bundle. It returns an error when a required field (target)
// ends up empty or when reserved profile names slip through.
func Resolve(cmd *cobra.Command, g *GlobalFlags) (Resolved, error) {
	r := Resolved{
		Logger:     logging.Setup(g.Quiet, g.Verbose),
		Provenance: make(map[ConfigKey]Provenance, 8),
	}

	cfgPath := g.Config
	configExplicit := changed(cmd, "config")
	if cfgPath == "" {
		cfgPath = config.DefaultFilename
	}
	r.ConfigPath = cfgPath
	cfg, exists, err := config.Load(cfgPath)
	if err != nil {
		return r, err
	}
	if configExplicit && !exists {
		return r, fmt.Errorf("config file not found: %s", cfgPath)
	}
	r.ConfigExists = exists
	setKeys, err := config.LoadKeys(cfgPath)
	if err != nil {
		return r, err
	}

	// Relative paths in the config file are interpreted relative to the
	// config file's directory, not to CWD. Flag overrides use CWD.
	configBase := "."
	if exists {
		abs, err := filepath.Abs(cfgPath)
		if err != nil {
			return r, fmt.Errorf("resolve config path: %w", err)
		}
		configBase = filepath.Dir(abs)
	}

	source, sourceProv, err := resolvePath(cmd, "source", cfg.Source, g.Source, configBase, setKeys["source"])
	if err != nil {
		return r, err
	}
	r.Provenance[KeySource] = sourceProv

	target, targetProv, err := resolvePath(cmd, "target", cfg.Target, g.Target, configBase, setKeys["target"])
	if err != nil {
		return r, err
	}
	r.Provenance[KeyTarget] = targetProv
	if target == "" {
		return r, fmt.Errorf("target is required (set in %s or pass --target)", cfgPath)
	}

	r.Provenance[KeyDotPrefix] = provenanceFromKey(setKeys["dot_prefix"])
	r.Provenance[KeyIgnore] = provenanceFromKey(setKeys["ignore"])
	r.Provenance[KeyTraverseHidden] = provenanceFromKey(setKeys["traverse_hidden"])
	r.Provenance[KeyRespectGitignore] = provenanceFromKey(setKeys["respect_gitignore"])
	r.Provenance[KeyContinueOnError] = provenanceFromKey(setKeys["continue_on_error"])

	profiles, profilesProv := resolveProfiles(cmd, g, cfg, exists)
	r.Provenance[KeyProfiles] = profilesProv
	if err := (config.Config{Profiles: profiles}).Validate(); err != nil {
		return r, err
	}

	continueOnError := cfg.ContinueOnError
	if changed(cmd, "continue") {
		continueOnError = g.Continue
		r.Provenance[KeyContinueOnError] = ProvFlag
	}
	r.ContinueOnError = continueOnError

	gitignoreIgn, err := maybeGitignore(cfg.RespectGitignore, source)
	if err != nil {
		return r, err
	}
	globIgn, err := discover.NewGlobIgnorer(cfg.Ignore)
	if err != nil {
		return r, err
	}
	ignorer := discover.NewChain(globIgn, gitignoreIgn)

	r.Settings = discover.Settings{
		SourceDir:        source,
		TargetDir:        target,
		DotPrefix:        cfg.DotPrefix,
		Profiles:         profiles,
		Ignore:           ignorer,
		TraverseHidden:   cfg.TraverseHidden,
		RespectGitignore: cfg.RespectGitignore,
	}
	return r, nil
}

// provenanceFromKey returns ProvConfig if the key was set in the loaded
// file or ProvDefault otherwise.
func provenanceFromKey(set bool) Provenance {
	if set {
		return ProvConfig
	}
	return ProvDefault
}

func resolveProfiles(cmd *cobra.Command, g *GlobalFlags, cfg config.Config, cfgExists bool) ([]string, Provenance) {
	if changed(cmd, "profiles") {
		return dedupe(g.Profiles), ProvFlag
	}
	if cfgExists {
		out := append([]string{}, cfg.Profiles...)
		if cfg.EnvProfiles != "" {
			out = append(out, splitCSV(os.Getenv(cfg.EnvProfiles))...)
		}
		return dedupe(out), ProvConfigEnv
	}
	if envVal := os.Getenv(DefaultEnvProfiles); envVal != "" {
		return dedupe(splitCSV(envVal)), ProvEnv
	}
	return nil, ProvDefault
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

func maybeGitignore(enabled bool, source string) (discover.Ignorer, error) {
	if !enabled {
		return discover.NoopIgnorer(), nil
	}
	return discover.NewGitignoreIgnorer(source)
}

// resolveRelative turns a relative path from the config file into one
// anchored at the config file's directory. Absolute paths and paths
// starting with ~ or $VAR are left alone for ExpandPath to handle.
func resolveRelative(p, base string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if strings.HasPrefix(p, "~") || strings.Contains(p, "$") {
		return p
	}
	return filepath.Join(base, p)
}

func resolvePath(cmd *cobra.Command, flag, cfgVal, flagVal, configBase string, cfgSet bool) (string, Provenance, error) {
	p := resolveRelative(cfgVal, configBase)
	prov := provenanceFromKey(cfgSet)
	if changed(cmd, flag) {
		p = flagVal
		prov = ProvFlag
	}
	expanded, err := discover.ExpandPath(p)
	if err != nil {
		return "", prov, fmt.Errorf("expand %s: %w", flag, err)
	}
	return expanded, prov, nil
}
