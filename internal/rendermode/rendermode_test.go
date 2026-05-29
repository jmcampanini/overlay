package rendermode

import (
	"testing"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
)

func TestForGroupDefaults(t *testing.T) {
	cases := []struct {
		format document.Format
		want   Mode
	}{
		{format: document.FormatJSON, want: ModeMerge},
		{format: document.FormatTOML, want: ModeMerge},
		{format: document.FormatCopy, want: ModeCopy},
	}
	for _, tc := range cases {
		got, err := ForGroup(discover.Group{Format: tc.format}, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("ForGroup(%s) = %s, want %s", tc.format, got, tc.want)
		}
	}
}

func TestForGroupMatchesTargetRelativePath(t *testing.T) {
	got, err := ForGroup(discover.Group{
		Format:        document.FormatCopy,
		TargetRelPath: ".ssh/config",
	}, "/tmp/out", []config.RenderRule{{Path: ".ssh/config", Strategy: config.RenderStrategyAppend}})
	if err != nil {
		t.Fatal(err)
	}
	if got != ModeAppend {
		t.Errorf("mode = %s, want append", got)
	}
}

func TestForGroupUnmatchedRuleFallsBackToDefault(t *testing.T) {
	got, err := ForGroup(discover.Group{
		Format:        document.FormatCopy,
		TargetRelPath: ".npmrc",
	}, "/tmp/out", []config.RenderRule{{Path: "dot-npmrc", Strategy: config.RenderStrategyAppend}})
	if err != nil {
		t.Fatal(err)
	}
	if got != ModeCopy {
		t.Errorf("mode = %s, want copy", got)
	}
}
