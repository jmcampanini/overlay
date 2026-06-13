// Package rendermode maps discovered groups and user render rules to modes.
package rendermode

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
)

// Mode is the user-facing behavior used to render one target.
type Mode string

const (
	// ModeMerge structurally merges JSON, TOML, or YAML layers.
	ModeMerge Mode = "merge"
	// ModeCopy copies the highest-precedence active layer.
	ModeCopy Mode = "copy"
	// ModeAppend appends active layers in Overlay layer order.
	ModeAppend Mode = "append"
)

// String returns the display value for m.
func (m Mode) String() string {
	if m == "" {
		return "unknown"
	}
	return string(m)
}

// Decision is the resolved per-target rendering behavior: how layers are
// composed and whether variable substitution applies.
type Decision struct {
	Mode       Mode
	Substitute bool
}

// Decide returns the render decision for g after applying matching rules.
// globalSubstitute is the configuration-wide substitution switch; a non-nil
// substituteExclude opts the target back out when its target-relative path
// matches one of the exclude globs.
func Decide(g discover.Group, targetDir string, rules []config.RenderRule, globalSubstitute bool, substituteExclude discover.Ignorer) (Decision, error) {
	if substituteExclude == nil {
		substituteExclude = discover.NoopIgnorer()
	}
	decision := Decision{Mode: DefaultForFormat(g.Format), Substitute: globalSubstitute}
	if len(rules) == 0 && !decision.Substitute {
		return decision, nil
	}

	targetRel, err := targetRelativePath(g, targetDir)
	if err != nil {
		return Decision{}, err
	}
	normalizedTarget, err := config.NormalizeRenderRulePath(targetRel)
	if err != nil {
		return Decision{}, fmt.Errorf("normalize target path %q: %w", targetRel, err)
	}

	decision.Mode, err = modeForTarget(g.Format, decision.Mode, rules, normalizedTarget)
	if err != nil {
		return Decision{}, err
	}

	if decision.Substitute && substituteExclude.Match(normalizedTarget, false) {
		decision.Substitute = false
	}
	return decision, nil
}

// modeForTarget resolves the render mode for normalizedTarget, returning
// fallback unchanged when no rule matches.
func modeForTarget(format document.Format, fallback Mode, rules []config.RenderRule, normalizedTarget string) (Mode, error) {
	if len(rules) == 0 {
		return fallback, nil
	}
	normalizedRules, err := config.NormalizeRenderRules(rules)
	if err != nil {
		return "", err
	}
	for _, rule := range normalizedRules {
		if rule.Path != normalizedTarget {
			continue
		}
		switch rule.Strategy {
		case config.RenderStrategyAppend:
			return ModeAppend, nil
		case config.RenderStrategyCopy:
			return ModeCopy, nil
		case config.RenderStrategyMerge:
			if DefaultForFormat(format) != ModeMerge {
				return "", fmt.Errorf("render rule for %q names strategy %q but the format is not mergeable (json/toml/yaml)", rule.Path, rule.Strategy)
			}
			return fallback, nil
		case "":
			return fallback, nil
		default:
			return "", fmt.Errorf("unsupported render rule strategy %q for %q", rule.Strategy, rule.Path)
		}
	}
	return fallback, nil
}

// DefaultForFormat returns Overlay's default mode for a discovered format.
func DefaultForFormat(f document.Format) Mode {
	switch f {
	case document.FormatJSON, document.FormatTOML, document.FormatYAML:
		return ModeMerge
	default:
		return ModeCopy
	}
}

func targetRelativePath(g discover.Group, targetDir string) (string, error) {
	if g.TargetRelPath != "" {
		return g.TargetRelPath, nil
	}
	if targetDir == "" {
		return "", fmt.Errorf("target directory is required to match render rules for %q", g.TargetPath)
	}
	rel, err := filepath.Rel(targetDir, g.TargetPath)
	if err != nil {
		return "", fmt.Errorf("make %q relative to %q: %w", g.TargetPath, targetDir, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("target path %q is outside target directory %q", g.TargetPath, targetDir)
	}
	return filepath.ToSlash(rel), nil
}
