package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

type colorMode string

const (
	colorAuto   colorMode = "auto"
	colorAlways colorMode = "always"
	colorNever  colorMode = "never"
)

// colorValue binds --color to a colorMode and rejects other values.
type colorValue struct {
	mode *colorMode
}

func (v colorValue) Set(value string) error {
	switch mode := colorMode(value); mode {
	case colorAuto, colorAlways, colorNever:
		*v.mode = mode
		return nil
	default:
		return fmt.Errorf("invalid color mode %q: must be auto, always, or never", value)
	}
}

func (v colorValue) String() string { return string(*v.mode) }

func (colorValue) Type() string { return "mode" }

// resolveColor picks the color profile for one output stream. An explicit
// --color value wins: never disables color, always guarantees at least basic
// ANSI color, and auto uses the detected profile. Without an explicit value a
// nonempty NO_COLOR disables color and the detected profile decides otherwise.
func resolveColor(mode colorMode, explicit bool, noColor bool, detected colorprofile.Profile) colorprofile.Profile {
	switch {
	case explicit && mode == colorNever:
		return colorprofile.ASCII
	case explicit && mode == colorAlways:
		return max(detected, colorprofile.ANSI)
	case explicit:
		return detected
	case noColor:
		return colorprofile.ASCII
	default:
		return detected
	}
}

// detectColor returns the profile the destination supports on its own:
// NoTTY when out is not a terminal, NoTTY when TERM is dumb, and otherwise
// the profile TERM and COLORTERM describe. Overrides are applied by
// resolveColor, so the variables that would preempt detection are removed
// from environ first.
func detectColor(out io.Writer, environ []string) colorprofile.Profile {
	return colorprofile.Detect(out, detectionEnviron(environ))
}

// detectionEnviron drops the variables colorprofile would otherwise treat as
// overrides: NO_COLOR is resolveColor's job, the CLICOLOR family is not part
// of overlay's contract, and TMUX makes detection run tmux without a timeout.
func detectionEnviron(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		switch key, _, _ := strings.Cut(entry, "="); key {
		case "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "TMUX":
			continue
		default:
			kept = append(kept, entry)
		}
	}
	return kept
}

func colorEnabled(profile colorprofile.Profile) bool {
	return profile > colorprofile.ASCII
}
