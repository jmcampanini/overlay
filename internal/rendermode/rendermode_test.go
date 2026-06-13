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

func TestDecideSubstituteGating(t *testing.T) {
	cases := []struct {
		name           string
		format         document.Format
		targetRel      string
		rules          []config.RenderRule
		global         bool
		exclude        []string
		wantSubstitute bool
		wantMode       Mode
	}{
		{
			name:      "exact path opts out even with global on",
			format:    document.FormatCopy,
			targetRel: ".config/shell/theme.sh",
			global:    true,
			exclude:   []string{".config/shell/theme.sh"},
			wantMode:  ModeCopy,
		},
		{
			name:      "glob opts out a whole subtree",
			format:    document.FormatCopy,
			targetRel: ".config/shell/aliases.sh",
			global:    true,
			exclude:   []string{".config/shell/**"},
			wantMode:  ModeCopy,
		},
		{
			name:      "exclude is inert when substitution is globally off",
			format:    document.FormatCopy,
			targetRel: ".config/shell/theme.sh",
			global:    false,
			exclude:   []string{".config/shell/theme.sh"},
			wantMode:  ModeCopy,
		},
		{
			name:           "target outside the exclude globs still substitutes",
			format:         document.FormatCopy,
			targetRel:      ".config/ghostty/config",
			global:         true,
			exclude:        []string{".config/shell/**"},
			wantSubstitute: true,
			wantMode:       ModeCopy,
		},
		{
			// A target can match a render rule (for its strategy) and an exclude
			// glob (to opt out of substitution) at once; the rule loop must fall
			// through to the exclude check rather than return early. FormatYAML
			// proves the rule's copy strategy overrode the merge default.
			name:      "rule strategy and exclude both apply",
			format:    document.FormatYAML,
			targetRel: ".config/lazygit/config.yml",
			rules:     []config.RenderRule{{Path: ".config/lazygit/config.yml", Strategy: config.RenderStrategyCopy}},
			global:    true,
			exclude:   []string{".config/lazygit/**"},
			wantMode:  ModeCopy,
		},
		{
			// A slashless pattern matches by base name at any depth (inherited
			// from the ignore field's glob semantics).
			name:      "slashless pattern matches nested target by basename",
			format:    document.FormatCopy,
			targetRel: ".config/shell/theme.sh",
			global:    true,
			exclude:   []string{"theme.sh"},
			wantMode:  ModeCopy,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := discover.Group{Format: tc.format, TargetRelPath: tc.targetRel}
			got, err := Decide(g, "/tmp/out", tc.rules, tc.global, excludeMatcher(t, tc.exclude...))
			if err != nil {
				t.Fatal(err)
			}
			if got.Substitute != tc.wantSubstitute {
				t.Errorf("Substitute = %v, want %v", got.Substitute, tc.wantSubstitute)
			}
			if got.Mode != tc.wantMode {
				t.Errorf("Mode = %s, want %s", got.Mode, tc.wantMode)
			}
		})
	}
}
