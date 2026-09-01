package diff

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/log/v2"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/render"
	"github.com/jmcampanini/overlay/internal/substitute"
)

func silentLogger() *log.Logger {
	l := log.New(os.Stderr)
	l.SetLevel(log.FatalLevel)
	return l
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiffIdentical(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "settings.olay.base.json"), `{"a":1}`)

	// First render to produce the target file so diff has something to compare.
	if err := render.Run(render.Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		StatePath: filepath.Join(t.TempDir(), ".overlay.state.json"),
		Logger:    silentLogger(),
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: silentLogger(),
		Out:    &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if differ {
		t.Errorf("expected no diff, got output:\n%s", buf.String())
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got: %s", buf.String())
	}
}

func TestDiffMissingTarget(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "settings.olay.base.json"), `{"a":1}`)

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: silentLogger(),
		Out:    &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !differ {
		t.Error("expected differ = true")
	}
	out := buf.String()
	if !strings.Contains(out, "+") {
		t.Errorf("expected additions:\n%s", out)
	}
}

func TestDiffYAMLDeterministic(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.yaml"), "z: 1\na: 2\n")
	writeFile(t, filepath.Join(target, "config.yaml"), "z: 1\na: 2\n")

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: silentLogger(),
		Out:    &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !differ {
		t.Fatal("expected deterministic YAML render to differ from source key order")
	}
	out := buf.String()
	if !strings.Contains(out, "-z: 1") || !strings.Contains(out, "+z: 1") {
		t.Fatalf("expected rendered YAML ordering diff:\n%s", out)
	}
}

func TestDiffCopyThroughChanged(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "script.olay.base.sh"), "new\n")
	writeFile(t, filepath.Join(target, "script.sh"), "old\n")

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: silentLogger(),
		Out:    &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !differ {
		t.Fatal("expected differ = true")
	}
	out := buf.String()
	if !strings.Contains(out, "-old") || !strings.Contains(out, "+new") {
		t.Fatalf("expected copy-through diff markers:\n%s", out)
	}
}

func TestDiffAppendUsesRenderRules(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "rc.olay.base"), "base")
	writeFile(t, filepath.Join(src, "rc.olay.work"), "work")
	rules := []config.RenderRule{{Path: "rc", Strategy: config.RenderStrategyAppend}}

	settings := discover.Settings{
		SourceDirs: []string{src},
		TargetDir:  target,
		Profiles:   []string{"work"},
		Ignore:     discover.NoopIgnorer(),
	}
	if err := render.Run(render.Options{
		Settings:    settings,
		RenderRules: rules,
		StatePath:   filepath.Join(t.TempDir(), ".overlay.state.json"),
		Logger:      silentLogger(),
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	differ, err := Run(Options{Settings: settings, RenderRules: rules, Logger: silentLogger(), Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if differ {
		t.Fatalf("expected no diff when diff uses append rules, got:\n%s", buf.String())
	}
}

func TestDiffFailFast(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "bad.olay.base.json"), `not valid json`)
	writeFile(t, filepath.Join(src, "good.olay.base.json"), `{"ok":true}`)

	var buf bytes.Buffer
	_, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: silentLogger(),
		Out:    &buf,
	})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDiffContinueOnError(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "bad.olay.base.json"), `not valid json`)
	writeFile(t, filepath.Join(src, "good.olay.base.json"), `{"ok":true}`)

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		ContinueOnError: true,
		Logger:          silentLogger(),
		Out:             &buf,
	})
	if err == nil {
		t.Error("expected summary error for failed file")
	}
	if !differ {
		t.Error("good file should have produced a diff against missing target")
	}
	if !strings.Contains(buf.String(), "+") {
		t.Errorf("expected diff for good file:\n%s", buf.String())
	}
}

func TestDiffChanged(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "settings.olay.base.json"), `{"a":1}`)
	// Pre-populate target with different content.
	writeFile(t, filepath.Join(target, "settings.json"), `{
  "a": 2
}
`)

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: silentLogger(),
		Out:    &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !differ {
		t.Error("expected differ = true")
	}
	out := buf.String()
	if !strings.Contains(out, "-") || !strings.Contains(out, "+") {
		t.Errorf("expected diff markers:\n%s", out)
	}
}

func TestDiffMissingVarsErrors(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "theme.olay.base.conf"), "bg=${PRE_GONE}\n")

	var buf bytes.Buffer
	_, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: substitute.NewResolver([]string{"PRE_*"}, nil, nil),
		Logger:      silentLogger(),
		Out:         &buf,
	})
	if err == nil {
		t.Fatal("missing vars should fail diff")
	}
	if !strings.Contains(err.Error(), "PRE_GONE") {
		t.Errorf("error should name the missing variable: %v", err)
	}
}

func TestDiffComparesSubstitutedContent(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "theme.olay.base.conf"), "bg=${PRE_BG}\n")
	writeFile(t, filepath.Join(target, "theme.conf"), "bg=dark\n")

	run := func(value string) (bool, string) {
		t.Helper()
		var buf bytes.Buffer
		differ, err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			Substituter: substitute.NewResolver([]string{"PRE_*"}, map[string]string{"PRE_BG": value}, nil),
			Logger:      silentLogger(),
			Out:         &buf,
		})
		if err != nil {
			t.Fatal(err)
		}
		return differ, buf.String()
	}

	if differ, out := run("dark"); differ {
		t.Errorf("substituted content matches disk, expected no diff:\n%s", out)
	}
	differ, out := run("light")
	if !differ {
		t.Error("changed pin should produce drift")
	}
	if !strings.Contains(out, "+bg=light") {
		t.Errorf("diff should show substituted value:\n%s", out)
	}
}

func TestDiffContinueOnComposeFailureDiffsCleanTargets(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "broken.olay.base.conf"), "bg=${PRE_GONE}\n")
	writeFile(t, filepath.Join(src, "clean.olay.base.conf"), "fg=${PRE_FG}\n")
	writeFile(t, filepath.Join(target, "clean.conf"), "fg=old\n")

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		ContinueOnError: true,
		Substituter:     substitute.NewResolver([]string{"PRE_*"}, map[string]string{"PRE_FG": "new"}, nil),
		Logger:          silentLogger(),
		Out:             &buf,
	})
	if err == nil {
		t.Fatal("compose failure should still return an error under --continue")
	}
	if !differ {
		t.Error("the clean target differs from disk, expected a diff")
	}
	if !strings.Contains(buf.String(), "+fg=new") {
		t.Errorf("clean target should be diffed despite the sibling failure:\n%s", buf.String())
	}
}
