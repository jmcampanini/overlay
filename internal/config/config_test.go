package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	c, exists, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if exists {
		t.Error("expected exists = false")
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Errorf("expected Default(), got %+v", c)
	}
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
source = "./src"
target = "~/out"
profiles = ["work", "vpn"]
env_profiles = "DOTFILES_PROFILE"
continue_on_error = true
ignore = ["**/node_modules"]
traverse_hidden = true
respect_gitignore = true
`)
	c, exists, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !exists {
		t.Error("exists should be true")
	}
	if c.Source != "./src" {
		t.Errorf("Source = %q", c.Source)
	}
	if c.Target != "~/out" {
		t.Errorf("Target = %q", c.Target)
	}
	if !reflect.DeepEqual(c.Profiles, []string{"work", "vpn"}) {
		t.Errorf("Profiles = %v", c.Profiles)
	}
	if c.EnvProfiles != "DOTFILES_PROFILE" {
		t.Errorf("EnvProfiles = %q", c.EnvProfiles)
	}
	if !c.ContinueOnError {
		t.Error("ContinueOnError should be true")
	}
	if !reflect.DeepEqual(c.Ignore, []string{"**/node_modules"}) {
		t.Errorf("Ignore = %v", c.Ignore)
	}
	if !c.TraverseHidden {
		t.Error("TraverseHidden should be true")
	}
	if !c.RespectGitignore {
		t.Error("RespectGitignore should be true")
	}
}

func TestLoadMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `not = valid = toml`)
	if _, _, err := Load(path); err == nil {
		t.Error("expected parse error")
	}
}

func TestLoadRejectsReservedProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
profiles = ["base"]
`)
	_, _, err := Load(path)
	if err == nil {
		t.Fatal("expected error for reserved profile")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should mention 'reserved': %v", err)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
respect_gitigore = true
`)
	_, _, err := Load(path)
	if err == nil {
		t.Fatal("expected error for typo'd key")
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "respect_gitigore") {
		t.Errorf("error should mention unknown key: %v", err)
	}
}

func TestDefault(t *testing.T) {
	c := Default()
	if c.Source != "." {
		t.Errorf("Source default = %q", c.Source)
	}
	if !c.DotPrefix {
		t.Error("DotPrefix default should be true")
	}
	if c.Ignore == nil {
		t.Error("Ignore should be initialized")
	}
	if c.Profiles == nil {
		t.Error("Profiles should be initialized")
	}
	if c.Target != "" {
		t.Errorf("Target should have no default, got %q", c.Target)
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	// A file that only sets target should inherit every other default.
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `target = "~/"`)
	c, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Source != "." {
		t.Errorf("Source should default to \".\", got %q", c.Source)
	}
	if !c.DotPrefix {
		t.Error("DotPrefix should default to true")
	}
}

func TestLoadExplicitFalseOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
dot_prefix = false
`)
	c, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DotPrefix {
		t.Error("dot_prefix = false should override the default")
	}
}

func TestLoadKeysReportsTopLevelKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
traverse_hidden = false
`)
	keys, err := LoadKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	if !keys["target"] {
		t.Error("target should be in keys")
	}
	if !keys["traverse_hidden"] {
		t.Error("traverse_hidden should be in keys even when set to false")
	}
	if keys["source"] {
		t.Error("source should not be in keys")
	}
}

func TestLoadKeysMissingFile(t *testing.T) {
	keys, err := LoadKeys(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Errorf("missing file should yield empty keys, got %v", keys)
	}
}
