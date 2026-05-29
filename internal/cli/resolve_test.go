package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// setupCmd builds a dummy cobra command with the global flags bound,
// applies the given args, and returns the resulting command and flags.
func setupCmd(t *testing.T, args []string) (*cobra.Command, *GlobalFlags) {
	t.Helper()
	g := &GlobalFlags{}
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	g.Bind(cmd)
	cmd.SetArgs(args)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	return cmd, g
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

func TestResolveFlagsOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out", "--profiles", "a,b,c"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if r.Settings.TargetDir != "/tmp/out" {
		t.Errorf("Target = %q", r.Settings.TargetDir)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"a", "b", "c"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
	if r.Provenance.Profiles != ProvFlag {
		t.Errorf("ProfilesFrom = %v", r.Provenance.Profiles)
	}
}

func TestResolveConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["work"]
`)
	cmd, g := setupCmd(t, nil)
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"work"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
	if r.Provenance.Profiles != ProvConfig {
		t.Errorf("ProfilesFrom = %v, want config (no env contributed)", r.Provenance.Profiles)
	}
}

func TestResolveConfigPlusEnvProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["base_env_check"]
env_profiles = "TEST_EXTRA_PROFILES"
`)
	t.Setenv("TEST_EXTRA_PROFILES", "alpha,beta")
	cmd, g := setupCmd(t, nil)
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"base_env_check", "alpha", "beta"}
	if !reflect.DeepEqual(r.Settings.Profiles, want) {
		t.Errorf("Profiles = %v, want %v", r.Settings.Profiles, want)
	}
	// The headline guarantee: ProvConfigEnv only when env actually contributed.
	if r.Provenance.Profiles != ProvConfigEnv {
		t.Errorf("ProfilesProv = %v, want config+env", r.Provenance.Profiles)
	}
}

func TestResolveEnvProfilesDeclaredButUnsetIsConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["work"]
env_profiles = "TEST_NEVER_SET_ENV"
`)
	// TEST_NEVER_SET_ENV is intentionally not set.
	cmd, g := setupCmd(t, nil)
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"work"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
	// env_profiles is declared but contributed nothing -> ProvConfig, not ProvConfigEnv.
	if r.Provenance.Profiles != ProvConfig {
		t.Errorf("ProfilesProv = %v, want config (env_profiles unset, no contribution)", r.Provenance.Profiles)
	}
}

func TestResolveEnvProfilesDeclaredButEmptyIsConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["work"]
env_profiles = "TEST_EMPTY_CSV"
`)
	t.Setenv("TEST_EMPTY_CSV", " , , ")
	cmd, g := setupCmd(t, nil)
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	// env var is set but splitCSV strips empties, so nothing was contributed.
	if r.Provenance.Profiles != ProvConfig {
		t.Errorf("ProfilesProv = %v, want config (env var stripped to nothing)", r.Provenance.Profiles)
	}
}

func TestResolveFlagReplacesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["from_config"]
`)
	cmd, g := setupCmd(t, []string{"--profiles", "override"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"override"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
	if r.Provenance.Profiles != ProvFlag {
		t.Errorf("ProfilesFrom = %v", r.Provenance.Profiles)
	}
}

func TestResolveConfigBackedProfilesEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("OVERLAY_PROFILES", "auto1,auto2")
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"auto1", "auto2"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
	if r.Provenance.Profiles != ProvEnv {
		t.Errorf("ProfilesFrom = %v", r.Provenance.Profiles)
	}
}

func TestResolveMissingTargetErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, nil)
	if _, err := Resolve(cmd, g); err == nil {
		t.Error("expected error for missing target")
	}
}

func TestResolveReservedProfileErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out", "--profiles", "base"})
	if _, err := Resolve(cmd, g); err == nil {
		t.Error("expected error for reserved profile")
	}
}

func TestResolveRejectsInvalidRenderRule(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"

[[render_rules]]
path = ".npmrc"
strategy = "merge"
`)
	cmd, g := setupCmd(t, nil)
	_, err := Resolve(cmd, g)
	if err == nil {
		t.Fatal("expected invalid render rule error")
	}
	if !strings.Contains(err.Error(), "render_rules") || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention render_rules and unsupported strategy: %v", err)
	}
}

func TestResolveProfileDedupe(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out", "--profiles", "a,b,a,c,b"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(r.Settings.Profiles, want) {
		t.Errorf("Profiles = %v, want %v", r.Settings.Profiles, want)
	}
}

func TestResolveSourceRelativeFromConfig(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(sub, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["pkgs"]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(sub, "pkgs")}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
	if r.Provenance.Source != ProvConfig {
		t.Errorf("SourceFrom = %v, want config", r.Provenance.Source)
	}
}

func TestResolveMultipleSourcesRelativeFromConfig(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(sub, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["pi", "codex"]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(sub, "pi"), filepath.Join(sub, "codex")}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
	if !reflect.DeepEqual(r.SourceLabels, []string{"pi", "codex"}) {
		t.Errorf("SourceLabels = %v", r.SourceLabels)
	}
}

func TestResolveSourceFlagOverride(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["from-config"]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{
		"--config", cfgPath,
		"--source", "/absolute/override",
	})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/absolute/override"}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
	if r.Provenance.Source != ProvFlag {
		t.Errorf("SourceFrom = %v, want flag", r.Provenance.Source)
	}
}

func TestResolveRepeatedSourceFlagReplacesConfig(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["from-config"]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{
		"--config", cfgPath,
		"--source", "pi",
		"--source", "codex",
	})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pi", "codex"}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
	if r.Provenance.Source != ProvFlag {
		t.Errorf("SourceFrom = %v, want flag", r.Provenance.Source)
	}
}

func TestResolveCommaSourcesFlagReplacesConfig(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["from-config"]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{
		"--config", cfgPath,
		"--sources", "pi,codex",
	})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pi", "codex"}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
}

func TestResolvePositionalSourcesReplaceConfigAndUseConfigBase(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["from-config"]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	r, err := Resolve(cmd, g, "pi", "codex")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "pi"), filepath.Join(root, "codex")}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
	if !reflect.DeepEqual(r.SourceLabels, []string{"pi", "codex"}) {
		t.Errorf("SourceLabels = %v", r.SourceLabels)
	}
	if r.Provenance.Source != ProvFlag {
		t.Errorf("SourceFrom = %v, want flag", r.Provenance.Source)
	}
}

func TestResolvePositionalSourcesIgnoreConfiguredSources(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = [""]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	r, err := Resolve(cmd, g, "pi")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "pi")}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
}

func TestResolveRejectsEnvInjectedReservedProfile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
target = "/tmp/out"
env_profiles = "TEST_INJECT_RESERVED"
`)
	t.Setenv("TEST_INJECT_RESERVED", "local")
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	if _, err := Resolve(cmd, g); err == nil {
		t.Error("expected error when env_profiles injects a reserved profile")
	}
}

func TestResolveExplicitMissingConfigErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--config", filepath.Join(dir, "missing.toml"), "--target", "/tmp/out"})
	if _, err := Resolve(cmd, g); err == nil {
		t.Error("expected error for explicit --config pointing at missing file")
	}
}

func TestResolveContinueFlagFalseOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
target = "/tmp/out"
continue_on_error = true
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath, "--continue=false"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if r.ContinueOnError {
		t.Error("--continue=false should override continue_on_error = true in config")
	}
	if r.Provenance.ContinueOnError != ProvFlag {
		t.Errorf("ContinueFrom = %v, want flag", r.Provenance.ContinueOnError)
	}
}

func TestResolveExpandsSourceTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--source", "~/", "--target", "/tmp/out"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{home}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
}

func TestResolveExpandsSourceEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("OVERLAY_TEST_SRC", "/custom/src")
	cmd, g := setupCmd(t, []string{"--source", "$OVERLAY_TEST_SRC", "--target", "/tmp/out"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/custom/src"}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
	}
}

func TestResolveTargetRelativeFromConfig(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "nested", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(sub, ".overlay.toml")
	writeFile(t, cfgPath, `target = "out"`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(sub, "out")
	if r.Settings.TargetDir != want {
		t.Errorf("TargetDir = %q, want %q", r.Settings.TargetDir, want)
	}
}

func TestGlobalFlagsRegistered(t *testing.T) {
	cmd, _ := setupCmd(t, nil)
	for _, name := range []string{"config", "sources", "source", "target", "profiles", "continue", "quiet", "verbose"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected --%s to be registered", name)
		}
	}
	for _, name := range []string{"dot-prefix", "env-profiles", "ignore", "traverse-hidden", "respect-gitignore", "render-rules"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("expected --%s to be absent", name)
		}
	}
}

func TestResolveEnvironmentTaggedFields(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("OVERLAY_SOURCES", "/env/source")
	t.Setenv("OVERLAY_TARGET", "/env/target")
	t.Setenv("OVERLAY_PROFILES", "env-a,env-b")
	t.Setenv("OVERLAY_CONTINUE", "true")
	cmd, g := setupCmd(t, nil)
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	wantSources := []string{"/env/source"}
	if !reflect.DeepEqual(r.RawConfig.Sources, wantSources) || !reflect.DeepEqual(r.Settings.SourceDirs, wantSources) {
		t.Errorf("Sources raw/runtime = %v/%v", r.RawConfig.Sources, r.Settings.SourceDirs)
	}
	if r.RawConfig.Target != "/env/target" || r.Settings.TargetDir != "/env/target" {
		t.Errorf("Target raw/runtime = %q/%q", r.RawConfig.Target, r.Settings.TargetDir)
	}
	if !reflect.DeepEqual(r.RawConfig.Profiles, []string{"env-a", "env-b"}) {
		t.Errorf("Raw profiles = %v", r.RawConfig.Profiles)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"env-a", "env-b"}) {
		t.Errorf("Effective profiles = %v", r.Settings.Profiles)
	}
	if !r.ContinueOnError {
		t.Error("ContinueOnError should come from OVERLAY_CONTINUE")
	}
	if r.Provenance.Source != ProvEnv || r.Provenance.Target != ProvEnv || r.Provenance.Profiles != ProvEnv || r.Provenance.ContinueOnError != ProvEnv {
		t.Errorf("provenance = %+v, want env for tagged fields", r.Provenance)
	}
}

func TestResolveTomlOnlyEnvironmentVariablesDoNotLoad(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("OVERLAY_TARGET", "/env/target")
	t.Setenv("OVERLAY_DOT_PREFIX", "false")
	t.Setenv("OVERLAY_IGNORE", "**/node_modules")
	t.Setenv("OVERLAY_TRAVERSE_HIDDEN", "true")
	t.Setenv("OVERLAY_RESPECT_GITIGNORE", "true")
	t.Setenv("OVERLAY_ENV_PROFILES", "SOME_VAR")
	t.Setenv("OVERLAY_RENDER_RULES", "not-a-rule")
	cmd, g := setupCmd(t, nil)
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !r.RawConfig.DotPrefix {
		t.Error("OVERLAY_DOT_PREFIX should not load")
	}
	if len(r.RawConfig.Ignore) != 0 {
		t.Errorf("OVERLAY_IGNORE should not load, got %v", r.RawConfig.Ignore)
	}
	if r.RawConfig.TraverseHidden {
		t.Error("OVERLAY_TRAVERSE_HIDDEN should not load")
	}
	if r.RawConfig.RespectGitignore {
		t.Error("OVERLAY_RESPECT_GITIGNORE should not load")
	}
	if r.RawConfig.EnvProfiles != "" {
		t.Errorf("OVERLAY_ENV_PROFILES should not load, got %q", r.RawConfig.EnvProfiles)
	}
	if len(r.RawConfig.RenderRules) != 0 {
		t.Errorf("OVERLAY_RENDER_RULES should not load, got %v", r.RawConfig.RenderRules)
	}
}

func TestResolveFlagsOverrideFileAndEnvironmentRawConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["file-src"]
target = "file-target"
profiles = ["file-a"]
continue_on_error = true
`)
	t.Setenv("OVERLAY_SOURCES", "env-src")
	t.Setenv("OVERLAY_TARGET", "env-target")
	t.Setenv("OVERLAY_PROFILES", "env-a,env-b")
	t.Setenv("OVERLAY_CONTINUE", "true")
	cmd, g := setupCmd(t, []string{
		"--source", "flag-src",
		"--target", "flag-target",
		"--profiles", "flag-a,flag-b",
		"--continue=false",
	})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.RawConfig.Sources, []string{"flag-src"}) || r.RawConfig.Target != "flag-target" {
		t.Errorf("raw sources/target = %v/%q", r.RawConfig.Sources, r.RawConfig.Target)
	}
	if !reflect.DeepEqual(r.RawConfig.Profiles, []string{"flag-a", "flag-b"}) {
		t.Errorf("raw profiles = %v", r.RawConfig.Profiles)
	}
	if r.RawConfig.ContinueOnError {
		t.Error("--continue=false should override file/env true")
	}
	if r.Provenance.Source != ProvFlag || r.Provenance.Target != ProvFlag || r.Provenance.Profiles != ProvFlag || r.Provenance.ContinueOnError != ProvFlag {
		t.Errorf("provenance = %+v, want flag for overridden fields", r.Provenance)
	}
}

func TestResolveCLIProfilesPlusEnvProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
target = "/tmp/out"
env_profiles = "TEST_EXTRA_PROFILES"
`)
	t.Setenv("TEST_EXTRA_PROFILES", "work")
	cmd, g := setupCmd(t, []string{"--config", cfgPath, "--profiles", "personal"})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.RawConfig.Profiles, []string{"personal"}) {
		t.Errorf("raw profiles = %v", r.RawConfig.Profiles)
	}
	want := []string{"personal", "work"}
	if !reflect.DeepEqual(r.Settings.Profiles, want) {
		t.Errorf("effective profiles = %v, want %v", r.Settings.Profiles, want)
	}
}

func TestResolveEnvProfilesDedupePreservesFirstOccurrence(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["a", "b"]
env_profiles = "TEST_EXTRA_PROFILES"
`)
	t.Setenv("TEST_EXTRA_PROFILES", "b,c,a")
	cmd, g := setupCmd(t, nil)
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(r.Settings.Profiles, want) {
		t.Errorf("effective profiles = %v, want %v", r.Settings.Profiles, want)
	}
}

func TestPrintRawConfigReportsRawValues(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["pkgs"]
target = "~/out"
profiles = ["work"]
env_profiles = "DOTFILES_PROFILE"
`)
	t.Setenv("DOTFILES_PROFILE", "vpn")
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := printRawConfig(&buf, raw); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`sources = ["pkgs"]`,
		`target = "~/out"`,
		`profiles = ["work"]`,
		`env_profiles = "DOTFILES_PROFILE"`,
		"# provenance",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("raw config output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "vpn") {
		t.Fatalf("raw config output should not include effective env profile:\n%s", out)
	}
}

func TestPrintRawConfigDoesNotRequireTarget(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, nil)
	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := printRawConfig(&buf, raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `target = ""`) {
		t.Fatalf("raw config output should include empty raw target:\n%s", buf.String())
	}
}
