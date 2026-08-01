package orphans

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmcampanini/overlay/internal/state"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, root string) (Options, []Orphan)
	}{
		{
			name: "orphan",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				target := regularFile(t, root, "orphan")
				entry := state.Entry{Target: target, Source: filepath.Join(root, "source")}
				return Options{Entries: []state.Entry{entry}}, []Orphan{{Target: entry.Target, Source: entry.Source}}
			},
		},
		{
			name: "in plan",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				target := regularFile(t, root, "planned")
				entry := state.Entry{Target: target, Source: filepath.Join(root, "source")}
				return Options{
					Entries:     []state.Entry{entry},
					PlanTargets: map[string]struct{}{target: {}},
				}, nil
			},
		},
		{
			name: "missing",
			run: func(_ *testing.T, root string) (Options, []Orphan) {
				entry := state.Entry{Target: filepath.Join(root, "missing"), Source: filepath.Join(root, "source")}
				return Options{Entries: []state.Entry{entry}}, nil
			},
		},
		{
			name: "directory",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				target := filepath.Join(root, "directory")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatal(err)
				}
				entry := state.Entry{Target: target, Source: filepath.Join(root, "source")}
				return Options{Entries: []state.Entry{entry}}, nil
			},
		},
		{
			name: "symlink",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				destination := regularFile(t, root, "destination")
				target := filepath.Join(root, "symlink")
				if err := os.Symlink(destination, target); err != nil {
					t.Fatal(err)
				}
				entry := state.Entry{Target: target, Source: filepath.Join(root, "source")}
				return Options{Entries: []state.Entry{entry}}, nil
			},
		},
		{
			name: "narrowed selected source",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				target := regularFile(t, root, "selected")
				source := filepath.Join(root, "selected-source")
				entry := state.Entry{Target: target, Source: source}
				return Options{
					Entries:         []state.Entry{entry},
					SelectedSources: map[string]struct{}{source: {}},
					Narrowed:        true,
				}, []Orphan{{Target: entry.Target, Source: entry.Source}}
			},
		},
		{
			name: "narrowed unselected source",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				target := regularFile(t, root, "unselected")
				entry := state.Entry{Target: target, Source: filepath.Join(root, "unselected-source")}
				return Options{
					Entries:         []state.Entry{entry},
					SelectedSources: map[string]struct{}{filepath.Join(root, "other-source"): {}},
					Narrowed:        true,
				}, nil
			},
		},
		{
			name: "unnarrowed ignores source selection",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				target := regularFile(t, root, "unnarrowed")
				entry := state.Entry{Target: target, Source: filepath.Join(root, "unselected-source")}
				return Options{
					Entries:         []state.Entry{entry},
					SelectedSources: map[string]struct{}{filepath.Join(root, "other-source"): {}},
				}, []Orphan{{Target: entry.Target, Source: entry.Source}}
			},
		},
		{
			name: "removed source",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				target := regularFile(t, root, "removed-source-target")
				entry := state.Entry{Target: target, Source: filepath.Join(root, "removed-source")}
				return Options{
					Entries:         []state.Entry{entry},
					SelectedSources: map[string]struct{}{filepath.Join(root, "configured-source"): {}},
					Narrowed:        false,
				}, []Orphan{{Target: entry.Target, Source: entry.Source}}
			},
		},
		{
			name: "sorted by target",
			run: func(t *testing.T, root string) (Options, []Orphan) {
				second := state.Entry{Target: regularFile(t, root, "b"), Source: filepath.Join(root, "source-b")}
				first := state.Entry{Target: regularFile(t, root, "a"), Source: filepath.Join(root, "source-a")}
				return Options{Entries: []state.Entry{second, first}}, []Orphan{
					{Target: first.Target, Source: first.Source},
					{Target: second.Target, Source: second.Source},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, want := tt.run(t, t.TempDir())
			if got := Detect(opts); !reflect.DeepEqual(got, want) {
				t.Fatalf("Detect() = %#v, want %#v", got, want)
			}
		})
	}
}

func regularFile(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
