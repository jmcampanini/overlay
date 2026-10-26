package cli

import (
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/google/go-cmp/cmp"
)

func TestResolveColor(t *testing.T) {
	tests := []struct {
		name     string
		mode     colorMode
		noColor  bool
		detected colorprofile.Profile
		want     bool
	}{
		{name: "auto on a terminal", mode: colorAuto, detected: colorprofile.ANSI256, want: true},
		{name: "auto redirected", mode: colorAuto, detected: colorprofile.NoTTY, want: false},
		{name: "auto with NO_COLOR", mode: colorAuto, noColor: true, detected: colorprofile.TrueColor, want: false},
		{name: "always redirected", mode: colorAlways, detected: colorprofile.NoTTY, want: true},
		{name: "always with NO_COLOR", mode: colorAlways, noColor: true, detected: colorprofile.NoTTY, want: true},
		{name: "never on a terminal", mode: colorNever, detected: colorprofile.TrueColor, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorEnabled(resolveColor(tt.mode, tt.noColor, tt.detected))

			if got != tt.want {
				t.Errorf("resolveColor(%s, noColor=%t, %s) enabled = %t, want %t", tt.mode, tt.noColor, tt.detected, got, tt.want)
			}
		})
	}
}

func TestDetectionEnvironDropsOverrides(t *testing.T) {
	got := detectionEnviron([]string{"TERM=xterm-256color", "NO_COLOR=1", "CLICOLOR_FORCE=1", "CLICOLOR=0", "TMUX=/tmp/x", "COLORTERM=truecolor"})

	want := []string{"TERM=xterm-256color", "COLORTERM=truecolor"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("detectionEnviron mismatch (-want +got):\n%s", diff)
	}
}

func TestColorValueRejectsUnknownModes(t *testing.T) {
	mode := colorAuto
	value := colorValue{mode: &mode}

	if err := value.Set("sometimes"); err == nil {
		t.Error("Set(sometimes) error = nil, want rejection")
	}
	if err := value.Set("never"); err != nil || mode != colorNever {
		t.Errorf("Set(never) error = %v, mode = %s, want never", err, mode)
	}
}
