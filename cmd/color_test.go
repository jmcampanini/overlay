package cmd

import (
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/google/go-cmp/cmp"
)

func TestResolveColor(t *testing.T) {
	tests := []struct {
		name     string
		mode     colorMode
		explicit bool
		noColor  bool
		detected colorprofile.Profile
		want     bool
	}{
		{name: "auto on a terminal", mode: colorAuto, detected: colorprofile.ANSI256, want: true},
		{name: "auto redirected", mode: colorAuto, detected: colorprofile.NoTTY, want: false},
		{name: "auto with NO_COLOR", mode: colorAuto, noColor: true, detected: colorprofile.TrueColor, want: false},
		{name: "explicit auto ignores NO_COLOR", mode: colorAuto, explicit: true, noColor: true, detected: colorprofile.ANSI, want: true},
		{name: "always redirected", mode: colorAlways, explicit: true, detected: colorprofile.NoTTY, want: true},
		{name: "always with NO_COLOR", mode: colorAlways, explicit: true, noColor: true, detected: colorprofile.NoTTY, want: true},
		{name: "never on a terminal", mode: colorNever, explicit: true, detected: colorprofile.TrueColor, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorEnabled(resolveColor(tt.mode, tt.explicit, tt.noColor, tt.detected))

			if got != tt.want {
				t.Errorf("resolveColor(%s, explicit=%t, noColor=%t, %s) enabled = %t, want %t", tt.mode, tt.explicit, tt.noColor, tt.detected, got, tt.want)
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
