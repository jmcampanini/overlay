package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadValuesMergesTheDirectoryInNameOrderThenThePaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "20-b.toml"), "[theme]\nbg = \"#000000\"\nfonts = [\"b\"]\n")
	writeFile(t, filepath.Join(dir, "10-a.toml"), "[theme]\nname = \"gruvbox\"\nbg = \"#282828\"\nfonts = [\"a\"]\n")
	writeFile(t, filepath.Join(dir, ".hidden.toml"), "hidden = true\n")
	writeFile(t, filepath.Join(dir, "notes.txt"), "notes = true\n")
	writeFile(t, filepath.Join(dir, "sub.toml", "inner.toml"), "sub = true\n")
	writeFile(t, filepath.Join(dir, "archive.tmpl", "old.toml"), "archived = true\n")
	if err := os.Symlink(filepath.Join(dir, "10-a.toml"), filepath.Join(dir, "30-link.toml")); err != nil {
		t.Fatal(err)
	}
	extraDir := t.TempDir()
	writeFile(t, filepath.Join(extraDir, "editor.toml"), "editor = \"nvim\"\n")
	extraFile := filepath.Join(t.TempDir(), "override.values")
	writeFile(t, extraFile, "[theme]\nname = \"nord\"\n")

	got, err := LoadValues(ValuesOptions{Dir: dir, Paths: []string{extraDir, extraFile}})
	if err != nil {
		t.Fatalf("LoadValues() error = %v", err)
	}

	want := Values{
		Dir: dir,
		Document: map[string]any{
			"editor": "nvim",
			"theme":  map[string]any{"name": "nord", "bg": "#282828", "fonts": []any{"a", "b", "a"}},
		},
		Files: []string{
			filepath.Join(dir, "10-a.toml"),
			filepath.Join(dir, "20-b.toml"),
			filepath.Join(dir, "30-link.toml"),
			filepath.Join(extraDir, "editor.toml"),
			extraFile,
		},
		Origins: map[string]string{"editor": filepath.Join(extraDir, "editor.toml"), "theme": extraFile},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LoadValues() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadValuesWithoutFilesIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{name: "absent directory", dir: filepath.Join(t.TempDir(), "missing")},
		{name: "empty directory", dir: t.TempDir()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadValues(ValuesOptions{Dir: tt.dir})
			if err != nil {
				t.Fatalf("LoadValues() error = %v", err)
			}

			want := Values{Dir: tt.dir, Document: map[string]any{}, Origins: map[string]string{}}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("LoadValues() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadValuesResolvesPathsAgainstTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "theme.toml"), "theme = \"nord\"\n")
	t.Chdir(dir)

	got, err := LoadValues(ValuesOptions{Dir: filepath.Join(dir, "missing"), Paths: []string{"theme.toml"}})
	if err != nil {
		t.Fatalf("LoadValues() error = %v", err)
	}

	want, err := filepath.EvalSymlinks(filepath.Join(dir, "theme.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 {
		t.Fatalf("LoadValues().Files = %v, want one file", got.Files)
	}
	if resolved, err := filepath.EvalSymlinks(got.Files[0]); err != nil || resolved != want {
		t.Errorf("LoadValues().Files[0] = %q (%v), want %q", got.Files[0], err, want)
	}
}

func TestLoadValuesFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) ValuesOptions
		// want is a substring of the error, with DIR standing for the case's
		// directory.
		want string
	}{
		{name: "template in the directory", setup: func(t *testing.T, dir string) ValuesOptions {
			t.Helper()
			writeFile(t, filepath.Join(dir, "theme.toml.tmpl"), "")
			return ValuesOptions{Dir: dir}
		}, want: "theme.toml.tmpl: a values file cannot be a template"},
		{name: "template path", setup: func(t *testing.T, dir string) ValuesOptions {
			t.Helper()
			writeFile(t, filepath.Join(dir, "theme.tmpl"), "")
			return ValuesOptions{Paths: []string{filepath.Join(dir, "theme.tmpl")}}
		}, want: "theme.tmpl: a values file cannot be a template"},
		{name: "missing path", setup: func(_ *testing.T, dir string) ValuesOptions {
			return ValuesOptions{Paths: []string{filepath.Join(dir, "missing.toml")}}
		}, want: "--values: stat"},
		{name: "empty path", setup: func(_ *testing.T, _ string) ValuesOptions {
			return ValuesOptions{Paths: []string{""}}
		}, want: "--values must name a file or directory"},
		{name: "dangling link in the directory", setup: func(t *testing.T, dir string) ValuesOptions {
			t.Helper()
			if err := os.Symlink(filepath.Join(dir, "gone.toml"), filepath.Join(dir, "theme.toml")); err != nil {
				t.Fatal(err)
			}
			return ValuesOptions{Dir: dir}
		}, want: "check values file: stat"},
		{name: "file that does not parse", setup: func(t *testing.T, dir string) ValuesOptions {
			t.Helper()
			writeFile(t, filepath.Join(dir, "bad.toml"), "theme =\n")
			return ValuesOptions{Dir: dir}
		}, want: "load values: fragment DIR/bad.toml: parse TOML"},
		{name: "table meets a string", setup: func(t *testing.T, dir string) ValuesOptions {
			t.Helper()
			writeFile(t, filepath.Join(dir, "a.toml"), "theme = \"gruvbox\"\n")
			writeFile(t, filepath.Join(dir, "b.toml"), "[theme]\nname = \"gruvbox\"\n")
			return ValuesOptions{Dir: dir}
		}, want: "fragment DIR/b.toml: key theme: cannot merge a table over a string from fragment DIR/a.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			opts := tt.setup(t, dir)
			want := strings.ReplaceAll(tt.want, "DIR", dir)

			_, err := LoadValues(opts)

			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("LoadValues(%+v) error = %v, want containing %q", opts, err, want)
			}
		})
	}
}

func TestDefaultValuesDirIsBesideTheConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := DefaultValuesDir()
	if err != nil {
		t.Fatalf("DefaultValuesDir() error = %v", err)
	}

	if want := filepath.Join(dir, "overlay", "values"); got != want {
		t.Errorf("DefaultValuesDir() = %q, want %q", got, want)
	}
}
