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
			// A real group always carries TargetRelPath; substitution resolves
			// it to test exclusion even with no rules.
			got, err := Decide(discover.Group{Format: tc.format, TargetRelPath: "x"}, "", nil, global, nil)
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

func TestDecideUnsupportedStrategyErrors(t *testing.T) {
	_, err := Decide(discover.Group{
		Format:        document.FormatCopy,
		TargetRelPath: ".config/ghostty/config",
	}, "/tmp/out", []config.RenderRule{{Path: ".config/ghostty/config", Strategy: "merge"}}, false, nil)
	if err == nil {
		t.Fatal("an unsupported strategy should error")
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

func TestDecideRuleAndExcludeBothApply(t *testing.T) {
	// A target can match a render rule (for its strategy) and an exclude glob
	// (to opt out of substitution) at once; the rule loop must fall through to
	// the exclude check rather than return early.
	g := discover.Group{Format: document.FormatYAML, TargetRelPath: ".config/lazygit/config.yml"}
	rules := []config.RenderRule{{Path: ".config/lazygit/config.yml", Strategy: config.RenderStrategyCopy}}
	got, err := Decide(g, "/tmp/out", rules, true, excludeMatcher(t, ".config/lazygit/**"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeCopy {
		t.Errorf("mode = %s, want copy from the rule", got.Mode)
	}
	if got.Substitute {
		t.Error("a target matching both a rule and an exclude glob must opt out of substitution")
	}
}

func TestDecideSlashlessExcludeMatchesNestedTarget(t *testing.T) {
	// A slashless pattern matches by basename at any depth (inherited from the
	// ignore field's glob semantics).
	g := discover.Group{Format: document.FormatCopy, TargetRelPath: ".config/shell/theme.sh"}
	got, err := Decide(g, "/tmp/out", nil, true, excludeMatcher(t, "theme.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Substitute {
		t.Error("a slashless exclude pattern should match the nested target by basename")
	}
}
