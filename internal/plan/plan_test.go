package plan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
)

func TestRenderBasic(t *testing.T) {
	groups := []discover.Group{
		{
			Stem:       "settings",
			Format:     document.FormatJSON,
			TargetPath: "/tmp/out/.claude/settings.json",
			Layers: []discover.Layer{
				{Profile: "base"},
				{Profile: "work"},
				{Profile: "local"},
			},
		},
		{
			Stem:       "config",
			Format:     document.FormatTOML,
			TargetPath: "/tmp/out/.codex/config.toml",
			Layers: []discover.Layer{
				{Profile: "base"},
				{Profile: "local"},
			},
		},
	}
	var buf bytes.Buffer
	if err := Render(&buf, groups, []string{"work"}, []string{"./src"}, "/tmp/out"); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Active profiles: [work]") {
		t.Errorf("missing active profiles header:\n%s", out)
	}
	if !strings.Contains(out, "2 files will be generated") {
		t.Errorf("missing summary line:\n%s", out)
	}
	if !strings.Contains(out, "TARGET") || !strings.Contains(out, "MODE") || !strings.Contains(out, "LAYERS") {
		t.Errorf("missing column headers:\n%s", out)
	}
	if strings.Contains(out, "FORMAT") {
		t.Errorf("plan should display MODE, not FORMAT:\n%s", out)
	}
	if !strings.Contains(out, "merge") {
		t.Errorf("missing merge mode:\n%s", out)
	}
	if !strings.Contains(out, "settings.json") {
		t.Errorf("missing settings.json row:\n%s", out)
	}
	if !strings.Contains(out, "base, work, local") {
		t.Errorf("missing layer list:\n%s", out)
	}
}

func TestRenderCopyThroughShowsWinner(t *testing.T) {
	groups := []discover.Group{
		{
			Stem:       "tool",
			Format:     document.FormatCopy,
			TargetPath: "/tmp/out/bin/tool.sh",
			Layers: []discover.Layer{
				{Profile: "base"},
				{Profile: "work"},
			},
		},
	}
	var buf bytes.Buffer
	if err := Render(&buf, groups, []string{"work"}, []string{"./src"}, "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "copy") {
		t.Fatalf("missing copy format:\n%s", out)
	}
	if !strings.Contains(out, "base, work (winner: work)") {
		t.Fatalf("missing copy winner:\n%s", out)
	}
}

func TestRenderAppendRuleShowsAppendMode(t *testing.T) {
	groups := []discover.Group{
		{
			Stem:          "dot-npmrc",
			Format:        document.FormatCopy,
			TargetPath:    "/tmp/out/.npmrc",
			TargetRelPath: ".npmrc",
			Layers: []discover.Layer{
				{Profile: "base"},
				{Profile: "work"},
			},
		},
	}
	var buf bytes.Buffer
	err := RenderWithOptions(&buf, groups, []string{"work"}, []string{"./src"}, "/tmp/out", Options{
		RenderRules: []config.RenderRule{{Path: ".npmrc", Strategy: config.RenderStrategyAppend}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "append") {
		t.Fatalf("missing append mode:\n%s", out)
	}
	if strings.Contains(out, "winner") {
		t.Fatalf("append mode should not show a copy winner:\n%s", out)
	}
}

func TestRenderCopyOverrideShowsWinnerForJSON(t *testing.T) {
	groups := []discover.Group{
		{
			Stem:          "settings",
			Format:        document.FormatJSON,
			TargetPath:    "/tmp/out/settings.json",
			TargetRelPath: "settings.json",
			Layers: []discover.Layer{
				{Profile: "base"},
				{Profile: "work"},
			},
		},
	}
	var buf bytes.Buffer
	err := RenderWithOptions(&buf, groups, []string{"work"}, []string{"./src"}, "/tmp/out", Options{
		RenderRules: []config.RenderRule{{Path: "settings.json", Strategy: config.RenderStrategyCopy}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "copy") || !strings.Contains(out, "winner: work") {
		t.Fatalf("missing copy override/winner:\n%s", out)
	}
}

func TestRenderMultipleSourcesHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil, []string{}, []string{"pi", "codex"}, "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sources: pi, codex") {
		t.Errorf("missing sources header:\n%s", out)
	}
}

func TestRenderManySourcesHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil, []string{}, []string{"a", "b", "c", "d", "e"}, "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sources: 5 configured") {
		t.Errorf("missing compact sources header:\n%s", out)
	}
}

func TestRenderEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil, []string{}, []string{"."}, "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "0 files will be generated") {
		t.Errorf("missing summary:\n%s", out)
	}
}

func TestCollapseHomeExactHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := collapseHome(home); got != "~" {
		t.Errorf("collapseHome(home) = %q, want %q", got, "~")
	}
}

func TestCollapseHomeWithinHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	in := filepath.Join(home, "a", "b")
	want := "~/" + filepath.Join("a", "b")
	if got := collapseHome(in); got != want {
		t.Errorf("collapseHome(%q) = %q, want %q", in, got, want)
	}
}

func TestCollapseHomeOutsideHome(t *testing.T) {
	in := "/tmp/x/y"
	if got := collapseHome(in); got != in {
		t.Errorf("collapseHome(%q) = %q, want unchanged", in, got)
	}
}
