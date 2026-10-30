package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValuesPrintsTheMergedDocumentAndItsFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := valuesDir()
	writeFixtureFile(t, filepath.Join(dir, "10-theme.toml"), "[theme]\nname = \"gruvbox\"\nbg = \"#282828\"\n", 0o644)
	writeFixtureFile(t, filepath.Join(dir, "50-local.toml"), "[theme]\nbg = \"#000000\"\n", 0o644)
	extra := filepath.Join(t.TempDir(), "editor.toml")
	writeFixtureFile(t, extra, "editor = \"nvim\"\n\"two\\nlines\" = 1\n", 0o644)

	out, errOut, err := executeForTest(t, "values", "--values", extra, "--provenance")
	if err != nil {
		t.Fatalf("values error = %v, stderr = %s", err, errOut)
	}

	want := strings.Join([]string{
		`editor = "nvim"`,
		`"two\nlines" = 1`,
		``,
		`[theme]`,
		`bg = "#000000"`,
		`name = "gruvbox"`,
		``,
		`# Effective`,
		`# values_dir = "` + dir + `"`,
		`# loaded_files = ["` + filepath.Join(dir, "10-theme.toml") + `", "` + filepath.Join(dir, "50-local.toml") + `", "` + extra + `"]`,
		``,
		`# Provenance`,
		`# key           file`,
		`# editor        "` + extra + `"`,
		`# theme         "` + filepath.Join(dir, "50-local.toml") + `"`,
		`# "two\nlines"  "` + extra + `"`,
		``,
	}, "\n")
	if out != want {
		t.Errorf("values output mismatch:\n got:\n%s\nwant:\n%s", out, want)
	}
}

func TestValuesWithoutADirectoryIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out, errOut, err := executeForTest(t, "values")
	if err != nil {
		t.Fatalf("values error = %v, stderr = %s", err, errOut)
	}

	if want := "\n# Effective\n# values_dir = \"" + valuesDir() + "\"\n# loaded_files = []\n"; out != want {
		t.Errorf("values output = %q, want %q", out, want)
	}
}

func TestValuesFailsOnAConflictNamingBothFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := valuesDir()
	writeFixtureFile(t, filepath.Join(dir, "a.toml"), "theme = \"gruvbox\"\n", 0o644)
	writeFixtureFile(t, filepath.Join(dir, "b.toml"), "[theme]\nname = \"gruvbox\"\n", 0o644)

	out, _, err := executeForTest(t, "values")

	want := "fragment " + filepath.Join(dir, "b.toml") + ": key theme: cannot merge a table over a string from fragment " + filepath.Join(dir, "a.toml")
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("values error = %v, want containing %q", err, want)
	}
	if out != "" {
		t.Errorf("values wrote to stdout on failure:\n%s", out)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("values directory missing after failure: %v", err)
	}
}
