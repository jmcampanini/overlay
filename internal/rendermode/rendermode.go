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
	// ModeMerge structurally merges JSON or TOML layers.
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

// ForGroup returns the render mode for g after applying matching rules.
func ForGroup(g discover.Group, targetDir string, rules []config.RenderRule) (Mode, error) {
	if len(rules) == 0 {
		return DefaultForFormat(g.Format), nil
	}
	normalizedRules, err := config.NormalizeRenderRules(rules)
	if err != nil {
		return "", err
	}
	targetRel, err := targetRelativePath(g, targetDir)
	if err != nil {
		return "", err
	}
	normalizedTarget, err := config.NormalizeRenderRulePath(targetRel)
	if err != nil {
		return "", fmt.Errorf("normalize target path %q: %w", targetRel, err)
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
		default:
			return "", fmt.Errorf("unsupported render rule strategy %q for %q", rule.Strategy, rule.Path)
		}
	}
	return DefaultForFormat(g.Format), nil
}

// DefaultForFormat returns Overlay's default mode for a discovered format.
func DefaultForFormat(f document.Format) Mode {
	switch f {
	case document.FormatJSON, document.FormatTOML:
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
