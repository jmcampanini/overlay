package diff

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"

	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/render"
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
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
		},
		Logger: silentLogger(),
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	differ, err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
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
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
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

func TestDiffFailFast(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "bad.olay.base.json"), `not valid json`)
	writeFile(t, filepath.Join(src, "good.olay.base.json"), `{"ok":true}`)

	var buf bytes.Buffer
	_, err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
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
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
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
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
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
