package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmcampanini/go-config-loader/configloader"
)

func TestLoadMissingFile(t *testing.T) {
	c, exists, report, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if exists {
		t.Error("expected exists = false")
	}
	if len(report.LoadedFiles) != 0 {
		t.Errorf("LoadedFiles = %v, want none", report.LoadedFiles)
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Errorf("expected Default(), got %+v", c)
	}
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
sources = ["./src"]
target = "~/out"
profiles = ["work", "vpn"]
env_profiles = "DOTFILES_PROFILE"
continue_on_error = true
toml_indent_tables = true
ignore = ["**/node_modules"]
traverse_hidden = true
respect_gitignore = true

[[render_rules]]
path = ".npmrc"
strategy = "append"

[[render_rules]]
path = ".some/generated.json"
strategy = "copy"
`)
	c, exists, report, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !exists {
		t.Error("exists should be true")
	}
	if len(report.LoadedFiles) != 1 {
		t.Fatalf("LoadedFiles = %v, want one file", report.LoadedFiles)
	}
	if report.Updates["sources"] != report.LoadedFiles[0] {
		t.Errorf("sources provenance = %q, want loaded file", report.Updates["sources"])
	}
	if !reflect.DeepEqual(c.Sources, []string{"./src"}) {
		t.Errorf("Sources = %v", c.Sources)
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
	if !c.TOMLIndentTables {
		t.Error("TOMLIndentTables should be true")
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
	wantRules := []RenderRule{
		{Path: ".npmrc", Strategy: RenderStrategyAppend},
		{Path: ".some/generated.json", Strategy: RenderStrategyCopy},
	}
	if !reflect.DeepEqual(c.RenderRules, wantRules) {
		t.Errorf("RenderRules = %#v, want %#v", c.RenderRules, wantRules)
	}
}

func TestLoadRejectsSingularSourceKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
source = "./src"
target = "~/out"
`)
	_, _, _, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "source") {
		t.Errorf("error should mention unknown source key: %v", err)
	}
}

func TestLoadRejectsSingularProfileKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
profile = "work"
target = "~/out"
`)
	_, _, _, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "profile") {
		t.Errorf("error should mention unknown profile key: %v", err)
	}
}

func TestLoadMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `not = valid = toml`)
	if _, _, _, err := Load(path); err == nil {
		t.Error("expected parse error")
	}
}

func TestLoadDoesNotValidateReservedProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
profiles = ["base"]
`)
	c, _, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load should only load raw config: %v", err)
	}
	if !reflect.DeepEqual(c.Profiles, []string{"base"}) {
		t.Errorf("Profiles = %v", c.Profiles)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
respect_gitigore = true
`)
	_, _, _, err := Load(path)
	if err == nil {
		t.Fatal("expected error for typo'd key")
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "respect_gitigore") {
		t.Errorf("error should mention unknown key: %v", err)
	}
}

func TestDefault(t *testing.T) {
	c := Default()
	if !reflect.DeepEqual(c.Sources, []string{"."}) {
		t.Errorf("Sources default = %v", c.Sources)
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
	if c.RenderRules == nil {
		t.Error("RenderRules should be initialized")
	}
	if c.Target != "" {
		t.Errorf("Target should have no default, got %q", c.Target)
	}
	if c.TOMLIndentTables {
		t.Error("TOMLIndentTables default should be false")
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	// A file that only sets target should inherit every other default.
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `target = "~/"`)
	c, _, report, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Sources, []string{"."}) {
		t.Errorf("Sources should default to [\".\"], got %v", c.Sources)
	}
	if report.Updates["sources"] != configloader.SourceDefault {
		t.Errorf("sources provenance = %q, want default", report.Updates["sources"])
	}
	if !c.DotPrefix {
		t.Error("DotPrefix should default to true")
	}
}

func TestSchemaDocsDescribeRenderRules(t *testing.T) {
	for _, want := range []string{
		"[[render_rules]]",
		"path = \".npmrc\"",
		"strategy = \"append\"",
		".json/.toml/.yaml/.yml -> merge",
		"Valid rules that do not match",
	} {
		if !strings.Contains(SchemaDocs, want) {
			t.Fatalf("SchemaDocs missing %q", want)
		}
	}
}

func TestLoadExplicitFalseOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
dot_prefix = false
`)
	c, _, report, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DotPrefix {
		t.Error("dot_prefix = false should override the default")
	}
	if report.Updates["dotprefix"] != report.LoadedFiles[0] {
		t.Errorf("dotprefix provenance = %q, want loaded file", report.Updates["dotprefix"])
	}
}
