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

// ProfileSource names where the active profile set came from.
type ProfileSource int

// ProfileSource constants identify which resolution path produced the
// active profile set.
const (
	ProfileSourceNone       ProfileSource = iota // no profiles configured
	ProfileSourceFlag                            // --profiles CLI flag
	ProfileSourceConfig                          // .overlay.toml (possibly + env_profiles)
	ProfileSourceDefaultEnv                      // OVERLAY_PROFILES env var
)

func (p ProfileSource) String() string {
	switch p {
	case ProfileSourceFlag:
		return "flag"
	case ProfileSourceConfig:
		return "config+env"
	case ProfileSourceDefaultEnv:
		return "env"
	}
	return "default"
}

// Resolved is the fully-resolved set of settings after applying the
// config-file + env + flag precedence rules.
type Resolved struct {
	Settings        discover.Settings
	ContinueOnError bool
	Logger          *log.Logger

	// Provenance: which source each setting came from, used by
	// `overlay config` to print "# from:" annotations.
	ConfigPath           string
	ConfigExists         bool
	SourceFrom           string
	TargetFrom           string
	DotPrefixFrom        string
	ProfilesFrom         ProfileSource
	IgnoreFrom           string
	TraverseHiddenFrom   string
	RespectGitignoreFrom string
	ContinueFrom         string
}

// Resolve merges config file, env vars, and CLI flags into a Resolved
// settings bundle. It returns an error when a required field (target)
// ends up empty or when reserved profile names slip through.
func Resolve(cmd *cobra.Command, g *GlobalFlags) (Resolved, error) {
	var r Resolved
	r.Logger = logging.Setup(g.Quiet, g.Verbose)

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
		if abs, err := filepath.Abs(cfgPath); err == nil {
			configBase = filepath.Dir(abs)
		}
	}

	source, sourceFrom, err := resolvePath(cmd, "source", cfg.Source, g.Source, configBase, setKeys["source"])
	if err != nil {
		return r, err
	}
	r.SourceFrom = sourceFrom

	target, targetFrom, err := resolvePath(cmd, "target", cfg.Target, g.Target, configBase, setKeys["target"])
	if err != nil {
		return r, err
	}
	r.TargetFrom = targetFrom
	if target == "" {
		return r, fmt.Errorf("target is required (set in %s or pass --target)", cfgPath)
	}

	r.DotPrefixFrom = fromSource(setKeys["dot_prefix"])
	r.IgnoreFrom = fromSource(setKeys["ignore"])
	r.TraverseHiddenFrom = fromSource(setKeys["traverse_hidden"])
	r.RespectGitignoreFrom = fromSource(setKeys["respect_gitignore"])
	r.ContinueFrom = fromSource(setKeys["continue_on_error"])

	profiles, profilesFrom := resolveProfiles(cmd, g, cfg, exists)
	r.ProfilesFrom = profilesFrom
	for _, p := range profiles {
		if p == discover.ProfileBase || p == discover.ProfileLocal {
			return r, fmt.Errorf("profile name %q is reserved", p)
		}
	}

	continueOnError := cfg.ContinueOnError
	if changed(cmd, "continue") {
		continueOnError = g.Continue
		r.ContinueFrom = "flag"
	}
	r.ContinueOnError = continueOnError

	gitignoreIgn, err := maybeGitignore(cfg.RespectGitignore, source)
	if err != nil {
		return r, err
	}
	ignorer := discover.NewChain(
		discover.NewGlobIgnorer(cfg.Ignore),
		gitignoreIgn,
	)

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

// fromSource returns "config" if the key was explicitly set in the
// loaded file, or "default" otherwise.
func fromSource(set bool) string {
	if set {
		return "config"
	}
	return "default"
}

func resolveProfiles(cmd *cobra.Command, g *GlobalFlags, cfg config.Config, cfgExists bool) ([]string, ProfileSource) {
	if changed(cmd, "profiles") {
		return dedupe(g.Profiles), ProfileSourceFlag
	}
	if cfgExists {
		out := append([]string{}, cfg.Profiles...)
		if cfg.EnvProfiles != "" {
			out = append(out, splitCSV(os.Getenv(cfg.EnvProfiles))...)
		}
		return dedupe(out), ProfileSourceConfig
	}
	if envVal := os.Getenv(DefaultEnvProfiles); envVal != "" {
		return dedupe(splitCSV(envVal)), ProfileSourceDefaultEnv
	}
	return nil, ProfileSourceNone
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

func resolvePath(cmd *cobra.Command, flag, cfgVal, flagVal, configBase string, cfgSet bool) (string, string, error) {
	p := resolveRelative(cfgVal, configBase)
	from := fromSource(cfgSet)
	if changed(cmd, flag) {
		p = flagVal
		from = "flag"
	}
	expanded, err := discover.ExpandPath(p)
	if err != nil {
		return "", "", fmt.Errorf("expand %s: %w", flag, err)
	}
	return expanded, from, nil
}
