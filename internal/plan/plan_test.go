package plan

import (
	"bytes"
	"strings"
	"testing"

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
	if err := Render(&buf, groups, []string{"work"}, "./src", "/tmp/out"); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Active profiles: [work]") {
		t.Errorf("missing active profiles header:\n%s", out)
	}
	if !strings.Contains(out, "2 files will be generated") {
		t.Errorf("missing summary line:\n%s", out)
	}
	if !strings.Contains(out, "TARGET") || !strings.Contains(out, "FORMAT") || !strings.Contains(out, "LAYERS") {
		t.Errorf("missing column headers:\n%s", out)
	}
	if !strings.Contains(out, "settings.json") {
		t.Errorf("missing settings.json row:\n%s", out)
	}
	if !strings.Contains(out, "base, work, local") {
		t.Errorf("missing layer list:\n%s", out)
	}
}

func TestRenderEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil, []string{}, ".", "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "0 files will be generated") {
		t.Errorf("missing summary:\n%s", out)
	}
}

func TestCollapseHome(t *testing.T) {
	// Just make sure it doesn't crash on arbitrary input.
	if got := collapseHome("/some/absolute/path"); got == "" {
		t.Error("collapseHome should not return empty string")
	}
}
