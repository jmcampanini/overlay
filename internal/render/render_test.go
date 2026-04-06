package render

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/log"

	"github.com/jmcampanini/overlay/internal/discover"
)

func newTestLogger() *log.Logger {
	l := log.New(os.Stderr)
	l.SetLevel(log.FatalLevel)
	return l
}

func TestRunBasic(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "dot-claude", "settings.olay.base.json"), `{"a":1,"nested":{"b":2}}`)
	writeFile(t, filepath.Join(src, "dot-claude", "settings.olay.work.json"), `{"nested":{"c":3}}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			DotPrefix: true,
			Profiles:  []string{"work"},
			Ignore:    discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := filepath.Join(target, ".claude", "settings.json")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	// Expect keys alphabetized: a, nested (with b, c)
	want := `{
  "a": 1,
  "nested": {
    "b": 2,
    "c": 3
  }
}
`
	if string(data) != want {
		t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestRunScalarListDedupe(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "settings.olay.base.json"), `{"allow":["a","b"]}`)
	writeFile(t, filepath.Join(src, "settings.olay.work.json"), `{"allow":["b","c"]}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			Profiles:  []string{"work"},
			Ignore:    discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "settings.json"))
	want := `{
  "allow": [
    "a",
    "b",
    "c"
  ]
}
`
	if string(data) != want {
		t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestRunTOML(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.toml"), `model = "gpt-5.4"`)
	writeFile(t, filepath.Join(src, "config.olay.local.toml"), `
[projects."/path"]
trust_level = "trusted"
`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "config.toml"))
	if !contains(data, `model = 'gpt-5.4'`) && !contains(data, `model = "gpt-5.4"`) {
		t.Errorf("missing model: %s", data)
	}
	if !contains(data, "trust_level") {
		t.Errorf("missing trust_level: %s", data)
	}
}

func TestRunNoFilesFound(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Errorf("no files should not error, got: %v", err)
	}
}

func TestRunFailFast(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "bad.olay.base.json"), `not valid json`)
	writeFile(t, filepath.Join(src, "good.olay.base.json"), `{"ok":true}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRunContinueOnError(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "bad.olay.base.json"), `not valid json`)
	writeFile(t, filepath.Join(src, "good.olay.base.json"), `{"ok":true}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDir: src,
			TargetDir: target,
			Ignore:    discover.NoopIgnorer(),
		},
		ContinueOnError: true,
		Logger:          newTestLogger(),
	})
	if err == nil {
		t.Error("expected error summary")
	}
	if _, err := os.Stat(filepath.Join(target, "good.json")); err != nil {
		t.Errorf("good file should have been written: %v", err)
	}
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

func contains(data []byte, s string) bool {
	return len(data) > 0 && len(s) > 0 && indexOf(data, s) >= 0
}

func indexOf(haystack []byte, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == needle {
			return i
		}
	}
	return -1
}
