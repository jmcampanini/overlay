package rendermode

import (
	"testing"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
)

func excludeMatcher(t *testing.T, patterns ...string) discover.Ignorer {
	t.Helper()
	ign, err := discover.NewGlobIgnorer(patterns)
	if err != nil {
		t.Fatalf("NewGlobIgnorer: %v", err)
	}
	return ign
}

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
			got, err := Decide(discover.Group{Format: tc.format}, "", nil, global, nil)
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
	}, "/tmp/out", []config.RenderRule{{Path: ".ssh/config", Strategy: config.RenderStrategyAppend}}, false, nil)
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
	}, "/tmp/out", []config.RenderRule{{Path: "dot-npmrc", Strategy: config.RenderStrategyAppend}}, true, nil)
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
	}, "/tmp/out", []config.RenderRule{{Path: ".config/starship.toml"}}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeMerge {
		t.Errorf("mode = %s, want merge (format default)", got.Mode)
	}
	if !got.Substitute {
		t.Error("a rule with no exclude should inherit global substitute")
	}
}

func TestDecideExplicitMerge(t *testing.T) {
	got, err := Decide(discover.Group{
		Format:        document.FormatYAML,
		TargetRelPath: ".config/lazygit/config.yml",
	}, "/tmp/out", []config.RenderRule{{Path: ".config/lazygit/config.yml", Strategy: config.RenderStrategyMerge}}, false, nil)
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
	}, "/tmp/out", []config.RenderRule{{Path: ".config/ghostty/config", Strategy: config.RenderStrategyMerge}}, false, nil)
	if err == nil {
		t.Fatal("merge strategy on non-structured format should error")
	}
}

func TestDecideExcludeOptsOut(t *testing.T) {
	g := discover.Group{Format: document.FormatCopy, TargetRelPath: ".config/shell/theme.sh"}
	got, err := Decide(g, "/tmp/out", nil, true, excludeMatcher(t, ".config/shell/theme.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Substitute {
		t.Error("an excluded target should not substitute even with global on")
	}
}

func TestDecideExcludeGlobOptsOutSubtree(t *testing.T) {
	g := discover.Group{Format: document.FormatCopy, TargetRelPath: ".config/shell/aliases.sh"}
	got, err := Decide(g, "/tmp/out", nil, true, excludeMatcher(t, ".config/shell/**"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Substitute {
		t.Error("a target under an excluded glob should not substitute")
	}
}

func TestDecideExcludeInertWhenGlobalOff(t *testing.T) {
	g := discover.Group{Format: document.FormatCopy, TargetRelPath: ".config/shell/theme.sh"}
	got, err := Decide(g, "/tmp/out", nil, false, excludeMatcher(t, ".config/shell/theme.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Substitute {
		t.Error("substitution is globally off; exclude changes nothing")
	}
}

func TestDecideNonExcludedTargetStillSubstitutes(t *testing.T) {
	g := discover.Group{Format: document.FormatCopy, TargetRelPath: ".config/ghostty/config"}
	got, err := Decide(g, "/tmp/out", nil, true, excludeMatcher(t, ".config/shell/**"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Substitute {
		t.Error("a target outside the exclude globs should still substitute")
	}
}
