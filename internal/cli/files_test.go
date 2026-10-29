package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jmcampanini/overlay/internal/gittest"
)

type fileFixture struct {
	configPath string
	sources    []string
}

func newFileFixture(t *testing.T) fileFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OVERLAY_LAYERS", "")
	if err := os.Unsetenv("OVERLAY_LAYERS"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f := fileFixture{configPath: filepath.Join(t.TempDir(), "config.toml")}
	var configuration strings.Builder
	configuration.WriteString("layers = [\"work\", \"laptop\"]\n")
	for _, name := range []string{"dotfiles-base", "dotfiles-personal"} {
		path := filepath.Join(t.TempDir(), name)
		gittest.Run(t, filepath.Dir(path), "init", "-q", path)
		writeFixtureFile(t, filepath.Join(path, "overlay.toml"),
			"[layers.base]\n[layers.work]\n[layers.laptop]\n[layers.local]\n", 0o644)
		f.sources = append(f.sources, path)
		fmt.Fprintf(&configuration, "[[source]]\nurl = %q\npath = %q\n", "git@fixture:"+name+".git", path)
	}
	writeConfig(t, f.configPath, configuration.String())
	return f
}

func requireFileCommandOutput(t *testing.T, args []string, want string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := Execute(args, &out, &errOut)
	if status != 0 || out.String() != want || errOut.Len() != 0 {
		t.Errorf("Execute(%q) = status %d, stdout %q, stderr %q; want status 0, stdout %q, empty stderr",
			args, status, out.String(), errOut.String(), want)
	}
}

func requireFileCommandFailure(t *testing.T, args []string, wantStatus int, wantText ...string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := Execute(args, &out, &errOut)
	if status != wantStatus || out.Len() != 0 || !strings.HasPrefix(errOut.String(), "overlay: ") || strings.Count(errOut.String(), "\n") != 1 {
		t.Errorf("Execute(%q) = status %d, stdout %q, stderr %q; want status %d, empty stdout, one diagnostic line",
			args, status, out.String(), errOut.String(), wantStatus)
	}
	for _, text := range wantText {
		if !strings.Contains(errOut.String(), text) {
			t.Errorf("Execute(%q) stderr = %q, want containing %q", args, errOut.String(), text)
		}
	}
}

func TestFileCommandsOrderLayersSourcesAndPatterns(t *testing.T) {
	f := newFileFixture(t)
	for _, layer := range []string{"base", "work", "laptop", "local"} {
		writeFixtureFile(t, filepath.Join(f.sources[0], "z."+layer), "z."+layer+"\n", 0o644)
		writeFixtureFile(t, filepath.Join(f.sources[0], "a."+layer), "a."+layer+"\n", 0o644)
		writeFixtureFile(t, filepath.Join(f.sources[1], "b."+layer), "b."+layer+"\n", 0o644)
	}
	tests := []struct {
		name   string
		flags  []string
		layers []string
	}{
		{name: "configured", layers: []string{"base", "work", "laptop", "local"}},
		{name: "plural override", flags: []string{"--layers", "laptop"}, layers: []string{"base", "laptop", "local"}},
		{name: "singular override", flags: []string{"--layer", "laptop", "--layer", "work"}, layers: []string{"base", "laptop", "work", "local"}},
		{name: "combined last occurrence", flags: []string{"--layer", "work", "--layers", "work,laptop"}, layers: []string{"base", "laptop", "work", "local"}},
		{name: "empty", flags: []string{"--layers", ""}, layers: []string{"base", "local"}},
		{name: "unknown layer", flags: []string{"--layers", "absent"}, layers: []string{"base", "local"}},
	}
	for _, command := range []string{"files", "cat"} {
		for _, tt := range tests {
			t.Run(command+"/"+tt.name, func(t *testing.T) {
				var want strings.Builder
				for _, layer := range tt.layers {
					for i, name := range []string{"z.", "a.", "b."} {
						path := name + layer
						if command == "files" {
							path = filepath.Join(f.sources[i/2], path)
						}
						want.WriteString(path + "\n")
					}
				}
				args := append([]string{command, "--config", f.configPath}, tt.flags...)
				args = append(args, "z.{layer}", "b.{layer}", "a.{layer}")

				requireFileCommandOutput(t, args, want.String())
			})
		}
	}
}

func TestFileCommandsApplyEnvironmentLayersAfterFlags(t *testing.T) {
	f := newFileFixture(t)
	configuration, err := os.ReadFile(f.configPath)
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, f.configPath, "env_layers = [\"OVERLAY_TEST_FILE_LAYERS\"]\n"+string(configuration))
	t.Setenv("OVERLAY_LAYERS", "laptop")
	t.Setenv("OVERLAY_TEST_FILE_LAYERS", " work, laptop,work, ")
	writeFixtureFile(t, filepath.Join(f.sources[0], "p.work"), "work\n", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[1], "p.laptop"), "laptop\n", 0o644)

	for _, command := range []string{"files", "cat"} {
		want := "laptop\nwork\n"
		if command == "files" {
			want = filepath.Join(f.sources[1], "p.laptop") + "\n" + filepath.Join(f.sources[0], "p.work") + "\n"
		}
		requireFileCommandOutput(t, []string{command, "--config", f.configPath, "--layers", "", "p.{layer}"}, want)
	}
}

func TestFileCommandsRejectPatternShapesBeforeLoadingConfiguration(t *testing.T) {
	tests := []struct {
		pattern string
		rule    string
	}{
		{pattern: "", rule: "empty"},
		{pattern: "packages", rule: "contain {layer}"},
		{pattern: "/p.{layer}", rule: "relative"},
		{pattern: "~/p.{layer}", rule: "relative"},
		{pattern: "~other/p.{layer}", rule: "relative"},
		{pattern: "../p.{layer}", rule: ".. path segment"},
		{pattern: "p/{layer}/../q", rule: ".. path segment"},
		{pattern: "p.{layer}/..", rule: ".. path segment"},
		{pattern: "{other}.{layer}", rule: "braces"},
		{pattern: "{p.{layer}", rule: "braces"},
		{pattern: "p}.{layer}", rule: "braces"},
		{pattern: "*.{layer}", rule: "globs"},
		{pattern: "?.{layer}", rule: "globs"},
		{pattern: "[p].{layer}", rule: "globs"},
	}
	missingConfig := filepath.Join(t.TempDir(), "missing.toml")
	for _, command := range []string{"files", "cat"} {
		for _, tt := range tests {
			t.Run(command+"/"+tt.pattern, func(t *testing.T) {
				requireFileCommandFailure(t, []string{command, "--config", missingConfig, tt.pattern}, 2, strconv.Quote(tt.pattern), tt.rule)
			})
		}
	}
}

func TestFileCommandsMatchTypesAndRepeatedPlaceholders(t *testing.T) {
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "base", "p.base"), "content\n", 0o644)
	link := filepath.Join(f.sources[0], "link.base")
	if err := os.Symlink("base/p.base", link); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(f.sources[0], "dir.base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(f.sources[0], "broken.base")); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"files", "cat"} {
		want := "content\ncontent\n"
		if command == "files" {
			want = filepath.Join(f.sources[0], "base", "p.base") + "\n" + link + "\n"
		}
		requireFileCommandOutput(t, []string{command, "--config", f.configPath,
			"./{layer}//p.{layer}", "dir.{layer}", "missing.{layer}", "link.{layer}", "broken.{layer}"}, want)
	}
}

func TestFileCommandsKeepDuplicateFileMatches(t *testing.T) {
	f := newFileFixture(t)
	path := filepath.Join(f.sources[0], "p.base")
	writeFixtureFile(t, path, "content\n", 0o644)
	for _, command := range []string{"files", "cat"} {
		want := strings.Repeat("content\n", 3)
		if command == "files" {
			want = strings.Repeat(path+"\n", 3)
		}
		requireFileCommandOutput(t, []string{command, "--config", f.configPath, "p.{layer}", "./p.{layer}", "p.{layer}"}, want)
	}
}

func TestFileCommandsNoMatches(t *testing.T) {
	f := newFileFixture(t)
	for _, command := range []string{"files", "cat"} {
		requireFileCommandOutput(t, []string{command, "--config", f.configPath, "absent.{layer}"}, "")
	}
	requireFileCommandOutput(t, []string{"cat", "--list", "--config", f.configPath, "absent.{layer}"}, "")
}

func TestFileCommandsCollisionsLeaveStdoutEmpty(t *testing.T) {
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "first.base"), "earlier match\n", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[0], "p.work"), "first source\n", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[1], "p.work"), "second source\n", 0o644)
	for _, command := range []string{"files", "cat"} {
		requireFileCommandFailure(t, []string{command, "--config", f.configPath, "first.{layer}", "p.{layer}"}, 1,
			"p.{layer}", "layer work", "dotfiles-base", "dotfiles-personal")
	}
}

func TestFileCommandsSourceFailuresLeaveStdoutEmpty(t *testing.T) {
	for _, invalid := range []string{"missing", "file"} {
		t.Run(invalid, func(t *testing.T) {
			f := newFileFixture(t)
			writeFixtureFile(t, filepath.Join(f.sources[0], "p.base"), "earlier match\n", 0o644)
			if err := os.RemoveAll(f.sources[1]); err != nil {
				t.Fatal(err)
			}
			if invalid == "file" {
				writeFixtureFile(t, f.sources[1], "not a directory", 0o644)
			}
			for _, command := range []string{"files", "cat"} {
				requireFileCommandFailure(t, []string{command, "--config", f.configPath, "p.{layer}"}, 1, "dotfiles-personal")
			}
		})
	}
}

func TestFileCommandsOnlyMatchDeclaredLayers(t *testing.T) {
	type match struct {
		layer  string
		source int
	}
	tests := []struct {
		name      string
		manifests []string
		matches   []match
	}{
		{
			name: "source declarations including empty package lists",
			manifests: []string{
				"[layers.base]\npackages = [\"render-only\"]\n[layers.work]\npackages = []\n[layers.server]\n",
				"[layers.laptop]\n[layers.local]\n",
			},
			matches: []match{{layer: "base"}, {layer: "work"}, {layer: "laptop", source: 1}, {layer: "local", source: 1}},
		},
		{
			name:      "automatic layers must be declared",
			manifests: []string{"[layers.work]\n", "[layers.laptop]\n"},
			matches:   []match{{layer: "work"}, {layer: "laptop", source: 1}},
		},
		{
			name:      "empty manifests contribute nothing",
			manifests: []string{"", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFileFixture(t)
			for i, source := range f.sources {
				if err := os.Mkdir(filepath.Join(source, "render-only"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, filepath.Join(source, "overlay.toml"), tt.manifests[i], 0o644)
				for _, layer := range []string{"base", "work", "laptop", "server", "local"} {
					writeFixtureFile(t, filepath.Join(source, "p."+layer), fmt.Sprintf("%d/p.%s\n", i, layer), 0o644)
				}
			}
			for _, command := range []string{"files", "cat"} {
				var want strings.Builder
				for _, match := range tt.matches {
					line := fmt.Sprintf("%d/p.%s", match.source, match.layer)
					if command == "files" {
						line = filepath.Join(f.sources[match.source], "p."+match.layer)
					}
					want.WriteString(line + "\n")
				}

				requireFileCommandOutput(t, []string{command, "--config", f.configPath, "p.{layer}"}, want.String())
			}
		})
	}
}

func TestFileCommandsSkipUndeclaredLayersBeforeStat(t *testing.T) {
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "overlay.toml"), "", 0o644)
	if err := os.Symlink("p.base", filepath.Join(f.sources[0], "p.base")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.sources[1], "p.base")
	writeFixtureFile(t, path, "declared\n", 0o644)

	requireFileCommandOutput(t, []string{"files", "--config", f.configPath, "p.{layer}"}, path+"\n")
	requireFileCommandOutput(t, []string{"cat", "--config", f.configPath, "p.{layer}"}, "declared\n")
}

func TestFileCommandsManifestFailuresLeaveStdoutEmpty(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     string
	}{
		{name: "missing", want: "read manifest"},
		{name: "malformed", manifest: "[layers.", want: "read manifest"},
		{name: "unknown key", manifest: "unknown = true\n", want: "unknown keys"},
		{name: "invalid layer", manifest: "[layers.INVALID]\n", want: "layer name"},
		{name: "missing package in unselected layer", manifest: "[layers.server]\npackages = [\"absent\"]\n", want: "absent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFileFixture(t)
			writeFixtureFile(t, filepath.Join(f.sources[0], "p.base"), "earlier match\n", 0o644)
			manifest := filepath.Join(f.sources[1], "overlay.toml")
			if tt.name == "missing" {
				if err := os.Remove(manifest); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFixtureFile(t, manifest, tt.manifest, 0o644)
			}
			for _, command := range []string{"files", "cat"} {
				requireFileCommandFailure(t, []string{command, "--config", f.configPath, "p.{layer}"}, 1,
					"dotfiles-personal", "overlay.toml", tt.want)
			}
		})
	}
}

func TestFileCommandsStatFailuresLeaveStdoutEmpty(t *testing.T) {
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "p.base"), "earlier match\n", 0o644)
	if err := os.Symlink("p.local", filepath.Join(f.sources[0], "p.local")); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"files", "cat"} {
		requireFileCommandFailure(t, []string{command, "--config", f.configPath, "p.{layer}"}, 1, "p.local")
	}
}

func TestFilesRejectsNewlinesBeforeOutput(t *testing.T) {
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "p.base"), "first", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[0], "p\n.local"), "last", 0o644)

	requireFileCommandFailure(t, []string{"files", "--config", f.configPath, "p.{layer}", "p\n.{layer}"}, 1, "newline")
}

func TestFileCommandsAcceptGlobalColorWithoutAddingStyling(t *testing.T) {
	f := newFileFixture(t)
	path := filepath.Join(f.sources[0], "p.base")
	writeFixtureFile(t, path, "\x1b[31mred\x1b[0m\n", 0o644)
	t.Setenv("NO_COLOR", "1")
	for _, command := range []string{"files", "cat"} {
		want := "\x1b[31mred\x1b[0m\n"
		if command == "files" {
			want = path + "\n"
		}
		for _, args := range [][]string{
			{"--color", "always", command, "--config", f.configPath, "p.{layer}"},
			{command, "--color", "always", "--config", f.configPath, "p.{layer}"},
		} {
			requireFileCommandOutput(t, args, want)
		}
	}
}
