package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderAndDiffRejectPathsOutsideTarget(t *testing.T) {
	for _, command := range []string{"render", "diff"} {
		for _, escape := range []string{"dot segment", "parent symlink"} {
			t.Run(command+"/"+escape, func(t *testing.T) {
				f := newRenderFixture(t)
				base := baseSourcePath(t, f)
				parent := t.TempDir()
				target := filepath.Join(parent, "target")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(parent, "outside")
				writeFixtureFile(t, outside, "keep\n", 0o644)
				sourcePath := filepath.Join(base, "bash", "dot-.", "outside")
				if escape == "parent symlink" {
					if err := os.Symlink(parent, filepath.Join(target, "linked")); err != nil {
						t.Fatal(err)
					}
					sourcePath = filepath.Join(base, "bash", "linked", "outside")
				}
				writeFixtureFile(t, sourcePath, "replace\n", 0o644)

				out, _, err := executeForTest(t, command, "--config", f.configPath, "--target", target)

				if err == nil || !strings.Contains(err.Error(), "outside the target directory") {
					t.Errorf("%s error = %v, want a path escaping the target", command, err)
				}
				if out != "" {
					t.Errorf("%s stdout = %q, want no plan or patch", command, out)
				}
				if got, err := os.ReadFile(outside); err != nil || string(got) != "keep\n" {
					t.Errorf("outside content = %q (%v), want keep", got, err)
				}
				requireNoFile(t, filepath.Join(target, ".bashrc"))
			})
		}
	}
}

func TestRenderAndDiffPreserveActiveConfiguration(t *testing.T) {
	for _, command := range []string{"render", "diff"} {
		for _, location := range []string{"explicit", "default", "symlink", "case alias", "symlink case alias"} {
			t.Run(command+"/"+location, func(t *testing.T) {
				f := newRenderFixture(t)
				base := baseSourcePath(t, f)
				original, err := os.ReadFile(f.configPath)
				if err != nil {
					t.Fatal(err)
				}
				relative := "config.toml"
				if location == "default" {
					relative = filepath.Join(".config", "overlay", "config.toml")
					t.Setenv("XDG_CONFIG_HOME", filepath.Join(f.home, ".config"))
				}
				configPath := filepath.Join(f.home, relative)
				writeConfig(t, configPath, string(original))
				readPath := configPath
				if strings.HasPrefix(location, "symlink") {
					readPath = filepath.Join(f.home, "config-link.toml")
					if err := os.Symlink(configPath, readPath); err != nil {
						t.Fatal(err)
					}
				}
				if strings.HasSuffix(location, "case alias") {
					relative = strings.ToUpper(filepath.Base(readPath))
					if _, err := os.Lstat(filepath.Join(f.home, relative)); os.IsNotExist(err) {
						t.Skip("filesystem is case-sensitive")
					}
				}
				writeFixtureFile(t, filepath.Join(base, "bash", relative), "replace\n", 0o644)
				args := []string{command}
				if location != "default" {
					args = append(args, "--config", readPath)
				}

				out, _, err := executeForTest(t, args...)

				if err == nil || !strings.Contains(err.Error(), "a file overlay loaded") {
					t.Errorf("%s error = %v, want protected configuration file", command, err)
				}
				if out != "" {
					t.Errorf("%s stdout = %q, want no plan or patch", command, out)
				}
				if got, err := os.ReadFile(readPath); err != nil || string(got) != string(original) {
					t.Errorf("configuration content = %q (%v), want the original configuration", got, err)
				}
				requireNoFile(t, filepath.Join(f.home, ".ssh", "config"))
			})
		}
	}
}

func TestRenderAndDiffPreserveLoadedValuesFiles(t *testing.T) {
	for _, command := range []string{"render", "diff"} {
		t.Run(command, func(t *testing.T) {
			f := newRenderFixture(t)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(f.home, ".config"))
			valuesFile := filepath.Join(valuesDir(), "10-theme.toml")
			writeFixtureFile(t, valuesFile, "[theme]\nname = \"gruvbox\"\n", 0o644)
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-config", "overlay", "values", "10-theme.toml"), "replace\n", 0o644)

			out, _, err := executeForTest(t, command, "--config", f.configPath)

			if err == nil || !strings.Contains(err.Error(), "a file overlay loaded") {
				t.Errorf("%s error = %v, want protected values file", command, err)
			}
			if out != "" {
				t.Errorf("%s stdout = %q, want no plan or patch", command, out)
			}
			if got, err := os.ReadFile(valuesFile); err != nil || string(got) != "[theme]\nname = \"gruvbox\"\n" {
				t.Errorf("values file = %q (%v), want the original values", got, err)
			}
		})
	}
}

func TestRenderAllowsSymlinksWithinTarget(t *testing.T) {
	f := newRenderFixture(t)
	base := baseSourcePath(t, f)
	actual := filepath.Join(f.home, "actual")
	if err := os.Mkdir(actual, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, filepath.Join(f.home, "linked")); err != nil {
		t.Fatal(err)
	}
	targetAlias := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(f.home, targetAlias); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(base, "bash", "linked", "new", "file"), "rendered\n", 0o644)

	_, _, err := executeForTest(t, "render", "--config", f.configPath, "--target", targetAlias)

	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(actual, "new", "file")); err != nil || string(got) != "rendered\n" {
		t.Errorf("rendered file = %q (%v), want rendered", got, err)
	}
}
