package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveLoadRoundTripUsesCanonicalFormat(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, ".overlay.state.json")
	firstTarget := writeRegular(t, filepath.Join(dir, "a-target"))
	secondTarget := writeRegular(t, filepath.Join(dir, "z-target"))
	firstSource := filepath.Join(dir, "a-source")
	secondSource := filepath.Join(dir, "z-source")
	if err := os.WriteFile(statePath, []byte("old state"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries := []Entry{
		{Target: secondTarget, Source: secondSource},
		{Target: firstTarget, Source: firstSource},
	}
	if err := Save(statePath, entries); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	contents, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	wantContents := fmt.Sprintf(`{
  "entries": [
    {
      "target": %q,
      "source": %q
    },
    {
      "target": %q,
      "source": %q
    }
  ]
}
`, firstTarget, firstSource, secondTarget, secondSource)
	if string(contents) != wantContents {
		t.Fatalf("state contents =\n%s\nwant:\n%s", contents, wantContents)
	}

	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("state permissions = %04o, want 0644", got)
	}

	got, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []Entry{
		{Target: firstTarget, Source: firstSource},
		{Target: secondTarget, Source: secondSource},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadAcceptsEmptyBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".overlay.state.json")
	if err := os.WriteFile(path, []byte("{\"entries\":[]}"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Load() = %#v, want empty entries", entries)
	}
}

func TestLoadMissingReturnsSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".overlay.state.json")

	_, err := Load(path)
	if !errors.Is(err, ErrNotExist) {
		t.Fatalf("Load() error = %v, want ErrNotExist", err)
	}
}

func TestLoadRejectsInvalidStateWithRecoveryInstructions(t *testing.T) {
	tests := map[string]string{
		"malformed JSON":         `{`,
		"missing entries":        `{}`,
		"null entries":           `{"entries":null}`,
		"unknown top-level":      `{"entries":[],"version":1}`,
		"case-variant entries":   `{"Entries":[]}`,
		"duplicate entries":      `{"entries":[],"entries":[]}`,
		"unknown entry field":    `{"entries":[{"target":"/target","source":"/source","owner":"x"}]}`,
		"case-variant target":    `{"entries":[{"Target":"/target","source":"/source"}]}`,
		"duplicate target field": `{"entries":[{"target":"/one","target":"/two","source":"/source"}]}`,
		"trailing JSON":          `{"entries":[]} {}`,
		"empty target":           `{"entries":[{"target":"","source":"/source"}]}`,
		"relative target":        `{"entries":[{"target":"target","source":"/source"}]}`,
		"empty source":           `{"entries":[{"target":"/target","source":""}]}`,
		"relative source":        `{"entries":[{"target":"/target","source":"source"}]}`,
		"duplicate target paths": `{"entries":[{"target":"/target","source":"/one"},{"target":"/target","source":"/two"}]}`,
	}

	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".overlay.state.json")
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() error = nil")
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("Load() error = %q, want path %q", err, path)
			}
			if !strings.Contains(err.Error(), "delete it and re-run overlay render to establish a baseline") {
				t.Fatalf("Load() error = %q, want recovery instructions", err)
			}
		})
	}
}

func TestSaveGarbageCollectsNonRegularTargets(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, ".overlay.state.json")
	regular := writeRegular(t, filepath.Join(dir, "regular"))
	gone := writeRegular(t, filepath.Join(dir, "gone"))
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(dir, "directory")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	blockedParent := writeRegular(t, filepath.Join(dir, "blocked-parent"))
	blocked := filepath.Join(blockedParent, "target")
	symlink := filepath.Join(dir, "symlink")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}

	entries := []Entry{
		{Target: gone, Source: filepath.Join(dir, "gone-source")},
		{Target: directory, Source: filepath.Join(dir, "directory-source")},
		{Target: blocked, Source: filepath.Join(dir, "blocked-source")},
		{Target: symlink, Source: filepath.Join(dir, "symlink-source")},
		{Target: regular, Source: filepath.Join(dir, "regular-source")},
	}
	if err := Save(statePath, entries); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []Entry{{Target: regular, Source: filepath.Join(dir, "regular-source")}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	assertNoTemporaryFiles(t, dir)
}

func TestSaveCleansUpTemporaryFileWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	target := writeRegular(t, filepath.Join(dir, "target"))
	statePath := filepath.Join(dir, ".overlay.state.json")
	if err := os.Mkdir(statePath, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Save(statePath, []Entry{{Target: target, Source: filepath.Join(dir, "source")}})
	if err == nil {
		t.Fatal("Save() error = nil, want rename error")
	}
	assertNoTemporaryFiles(t, dir)
}

func TestMergeClaimsOverwriteByTarget(t *testing.T) {
	prior := []Entry{
		{Target: "/z", Source: "/old-z"},
		{Target: "/a", Source: "/old-a"},
	}
	claimed := []Entry{
		{Target: "/z", Source: "/new-z"},
		{Target: "/m", Source: "/new-m"},
	}

	got := Merge(prior, claimed)
	want := []Entry{
		{Target: "/a", Source: "/old-a"},
		{Target: "/m", Source: "/new-m"},
		{Target: "/z", Source: "/new-z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge() = %#v, want %#v", got, want)
	}
}

func writeRegular(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertNoTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".overlay.state.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary state files remain: %v", matches)
	}
}
