package rendermode

import (
	"testing"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
)

func TestDecideDefaults(t *testing.T) {
	cases := []struct {
		format document.Format
		want   Mode
	}{
		{format: document.FormatJSON, want: ModeMerge},
		{format: document.FormatTOML, want: ModeMerge},
		{format: document.FormatYAML, want: ModeMerge},
		{format: document.FormatCopy, want: ModeCopy},
	}
	for _, tc := range cases {
		for _, global := range []bool{false, true} {
			got, err := Decide(discover.Group{Format: tc.format}, "", nil, global)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tc.want {
				t.Errorf("Decide(%s).Mode = %s, want %s", tc.format, got.Mode, tc.want)
			}
			if got.Substitute != global {
				t.Errorf("Decide(%s).Substitute = %v, want global %v", tc.format, got.Substitute, global)
			}
		}
	}
}

func TestDecideMatchesTargetRelativePath(t *testing.T) {
	got, err := Decide(discover.Group{
		Format:        document.FormatCopy,
		TargetRelPath: ".ssh/config",
	}, "/tmp/out", []config.RenderRule{{Path: ".ssh/config", Strategy: config.RenderStrategyAppend}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeAppend {
		t.Errorf("mode = %s, want append", got.Mode)
	}
}

func TestDecideUnmatchedRuleFallsBackToDefault(t *testing.T) {
	got, err := Decide(discover.Group{
		Format:        document.FormatCopy,
		TargetRelPath: ".npmrc",
	}, "/tmp/out", []config.RenderRule{{Path: "dot-npmrc", Strategy: config.RenderStrategyAppend}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeCopy {
		t.Errorf("mode = %s, want copy", got.Mode)
	}
	if !got.Substitute {
		t.Error("unmatched rule should inherit global substitute")
	}
}

func TestDecideOptionalStrategyInheritsFormatDefault(t *testing.T) {
	got, err := Decide(discover.Group{
		Format:        document.FormatTOML,
		TargetRelPath: ".config/starship.toml",
	}, "/tmp/out", []config.RenderRule{{Path: ".config/starship.toml", Substitute: config.TriStateTrue}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeMerge {
		t.Errorf("mode = %s, want merge (format default)", got.Mode)
	}
	if !got.Substitute {
		t.Error("substitute = true rule should override global off")
	}
}

func TestDecideExplicitMerge(t *testing.T) {
	got, err := Decide(discover.Group{
		Format:        document.FormatYAML,
		TargetRelPath: ".config/lazygit/config.yml",
	}, "/tmp/out", []config.RenderRule{{Path: ".config/lazygit/config.yml", Strategy: config.RenderStrategyMerge}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeMerge {
		t.Errorf("mode = %s, want merge", got.Mode)
	}
}

func TestDecideMergeOnNonStructuredFormatErrors(t *testing.T) {
	_, err := Decide(discover.Group{
		Format:        document.FormatCopy,
		TargetRelPath: ".config/ghostty/config",
	}, "/tmp/out", []config.RenderRule{{Path: ".config/ghostty/config", Strategy: config.RenderStrategyMerge}}, false)
	if err == nil {
		t.Fatal("merge strategy on non-structured format should error")
	}
}

func TestDecideSubstituteOverrides(t *testing.T) {
	rules := []config.RenderRule{{Path: ".config/shell/theme.sh", Substitute: config.TriStateFalse}}
	got, err := Decide(discover.Group{
		Format:        document.FormatCopy,
		TargetRelPath: ".config/shell/theme.sh",
	}, "/tmp/out", rules, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Substitute {
		t.Error("substitute = false rule should override global on")
	}
}
