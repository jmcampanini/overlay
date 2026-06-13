package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmcampanini/go-config-loader/configloader"
	"github.com/jmcampanini/go-config-loader/pflagloader"
	"github.com/spf13/cobra"
)

func newTestCmd() (*cobra.Command, *globalFlags) {
	g := &globalFlags{}
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	g.bindPersistentFlags(cmd)
	return cmd, g
}

// setupCmd builds a dummy cobra command with the global flags bound,
// applies the given args, and returns the resulting command and flags.
func setupCmd(t *testing.T, args []string) (*cobra.Command, *globalFlags) {
	t.Helper()
	cmd, g := newTestCmd()
	cmd.SetArgs(args)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	return cmd, g
}

func assertRawAndEffectiveProfiles(t *testing.T, r Resolved, want []string) {
	t.Helper()
	if !reflect.DeepEqual(r.RawConfig.Profiles, want) {
		t.Errorf("raw profiles = %v, want %v", r.RawConfig.Profiles, want)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, want) {
		t.Errorf("Profiles = %v, want %v", r.Settings.Profiles, want)
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

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q): %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			if err := os.Setenv(key, old); err != nil {
				t.Fatalf("restore env %s: %v", key, err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("clear env %s: %v", key, err)
		}
	})
}

func assertProvenanceRow(t *testing.T, out, path, value, source string) {
	t.Helper()
	want := []string{"#", path, value, source}
	for _, line := range strings.Split(out, "\n") {
		if reflect.DeepEqual(strings.Fields(line), want) {
			return
		}
	}
	t.Fatalf("provenance row %v not found:\n%s", want, out)
}

func TestResolveFlagsOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out", "--profiles", "a,b,c"})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if r.Settings.TargetDir != "/tmp/out" {
		t.Errorf("Target = %q", r.Settings.TargetDir)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"a", "b", "c"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"work"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"base_env_check", "alpha", "beta"}
	if !reflect.DeepEqual(r.Settings.Profiles, want) {
		t.Errorf("Profiles = %v, want %v", r.Settings.Profiles, want)
	}
}

func TestResolveEnvProfilesDeclaredButUnsetKeepsConfiguredProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["work"]
env_profiles = "TEST_NEVER_SET_ENV"
`)
	// TEST_NEVER_SET_ENV is intentionally not set.
	cmd, g := setupCmd(t, nil)
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"work"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
}

func TestResolveEnvProfilesDeclaredButEmptyKeepsConfiguredProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["work"]
env_profiles = "TEST_EMPTY_CSV"
`)
	t.Setenv("TEST_EMPTY_CSV", " , , ")
	cmd, g := setupCmd(t, nil)
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"work"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"override"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
}

func TestResolveSingularProfileFlagOverridesConfigAndEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
profiles = ["from_config"]
`)
	t.Setenv("OVERLAY_PROFILES", "from_env")
	cmd, g := setupCmd(t, []string{"--profile", "work"})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"work"}
	assertRawAndEffectiveProfiles(t, r, want)
}

func TestResolveRepeatedSingularProfileFlagPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out", "--profile", "work", "--profile", "personal"})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"work", "personal"}
	assertRawAndEffectiveProfiles(t, r, want)
}

func TestResolveMixedProfileFlagsCanonicalThenSingular(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{
		"--target", "/tmp/out",
		"--profiles", "work,personal",
		"--profile", "personal",
		"--profile", "client",
	})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"work", "personal", "client"}
	assertRawAndEffectiveProfiles(t, r, want)
}

func TestResolveRejectsEmptySingularProfileFlag(t *testing.T) {
	cmd, _ := newTestCmd()
	err := cmd.ParseFlags([]string{"--profile="})
	if err == nil {
		t.Fatal("expected --profile= to fail during flag parsing")
	}
	if !strings.Contains(err.Error(), "empty values") {
		t.Fatalf("error = %v, want empty value error", err)
	}
}

func TestResolveConfigBackedProfilesEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("OVERLAY_PROFILES", "auto1,auto2")
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out"})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"auto1", "auto2"}) {
		t.Errorf("Profiles = %v", r.Settings.Profiles)
	}
}

func TestResolveSingularProfileEnvironmentVariableIgnored(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	unsetEnv(t, "OVERLAY_PROFILES")
	t.Setenv("OVERLAY_TARGET", "/tmp/out")
	t.Setenv("OVERLAY_PROFILE", "work")
	cmd, g := setupCmd(t, nil)
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.RawConfig.Profiles) != 0 || len(r.Settings.Profiles) != 0 {
		t.Errorf("profiles raw/effective = %v/%v, want none", r.RawConfig.Profiles, r.Settings.Profiles)
	}
}

func TestResolveMissingTargetErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, nil)
	if _, err := resolve(cmd, g); err == nil {
		t.Error("expected error for missing target")
	}
}

func TestResolveReservedProfileErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--target", "/tmp/out", "--profiles", "base"})
	if _, err := resolve(cmd, g); err == nil {
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
strategy = "replace"
`)
	cmd, g := setupCmd(t, nil)
	_, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(sub, "pkgs")}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/absolute/override"}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pi", "codex"}
	if !reflect.DeepEqual(r.Settings.SourceDirs, want) {
		t.Errorf("SourceDirs = %v, want %v", r.Settings.SourceDirs, want)
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g, "pi", "codex")
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
}

func TestResolvePositionalSourcesIgnoreConfiguredSources(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = [""]
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	r, err := resolve(cmd, g, "pi")
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
	if _, err := resolve(cmd, g); err == nil {
		t.Error("expected error when env_profiles injects a reserved profile")
	}
}

func TestResolveExplicitMissingConfigErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, []string{"--config", filepath.Join(dir, "missing.toml"), "--target", "/tmp/out"})
	if _, err := resolve(cmd, g); err == nil {
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if r.ContinueOnError {
		t.Error("--continue=false should override continue_on_error = true in config")
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(sub, "out")
	if r.Settings.TargetDir != want {
		t.Errorf("TargetDir = %q, want %q", r.Settings.TargetDir, want)
	}
}

func TestRootFlagsRegistered(t *testing.T) {
	cmd, _ := setupCmd(t, nil)
	for _, name := range []string{"config", "sources", "source", "target", "profiles", "profile", "continue", "quiet", "verbose"} {
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
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
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(r.Settings.Profiles, want) {
		t.Errorf("effective profiles = %v, want %v", r.Settings.Profiles, want)
	}
}

func TestResolveRejectsInvalidIgnorePatternDuringEffectiveValidation(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
ignore = ["[bad"]
`)
	cmd, g := setupCmd(t, nil)
	_, err := resolve(cmd, g)
	if err == nil {
		t.Fatal("expected invalid ignore pattern error")
	}
	if !strings.Contains(err.Error(), `invalid ignore pattern "[bad"`) {
		t.Fatalf("error = %v, want invalid ignore pattern", err)
	}
}

func TestResolveCarriesNormalizedRenderRules(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"

[[render_rules]]
path = "./.npmrc"
strategy = "append"
`)
	cmd, g := setupCmd(t, nil)
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Effective.RenderRules) != 1 {
		t.Fatalf("render rules = %#v, want one rule", r.Effective.RenderRules)
	}
	got := r.Effective.RenderRules[0]
	if got.Path != ".npmrc" || got.Strategy != "append" {
		t.Fatalf("render rule = %#v, want normalized .npmrc append", got)
	}
}

func TestPrintConfigReportsRawAndEffectiveValues(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = ["pkgs"]
target = "out"
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
	if err := printConfig(&buf, raw); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`sources = ["pkgs"]`,
		`target = "out"`,
		`profiles = ["work"]`,
		`env_profiles = "DOTFILES_PROFILE"`,
		"# provenance",
		`# loaded_files = ["` + cfgPath + `"]`,
		`# effective_source_dirs = ["` + filepath.Join(dir, "pkgs") + `"]`,
		`# effective_target_dir = "` + filepath.Join(dir, "out") + `"`,
		`# effective_profiles = ["work", "vpn"]`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config output missing %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if line == `profiles = ["work", "vpn"]` {
			t.Fatalf("raw TOML should not include effective env profile:\n%s", out)
		}
	}
}

func TestPrintConfigProvenanceIncludesSourceColumn(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `profiles = ["file"]`)
	t.Setenv("OVERLAY_CONTINUE", "true")
	cmd, g := setupCmd(t, []string{"--config", cfgPath, "--target", "flag-target"})
	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := printConfig(&buf, raw); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertProvenanceRow(t, out, "Path", "Value", "Source")
	assertProvenanceRow(t, out, "profiles", `["file"]`, cfgPath)
	assertProvenanceRow(t, out, "continueonerror", "true", configloader.SourceEnv)
	assertProvenanceRow(t, out, "dotprefix", "true", configloader.SourceDefault)
	assertProvenanceRow(t, out, "target", `"flag-target"`, pflagloader.SourcePFlag)
}

func TestPrintConfigReportsEffectiveErrors(t *testing.T) {
	missingSourceEnv := "OVERLAY_TEST_MISSING_SOURCE"
	missingTargetEnv := "OVERLAY_TEST_MISSING_TARGET"
	unsetEnv(t, missingSourceEnv)
	unsetEnv(t, missingTargetEnv)

	tests := []struct {
		name string
		toml string
		env  map[string]string
		want []string
	}{
		{
			name: "empty sources",
			toml: `
sources = []
target = "/tmp/out"
`,
			want: []string{
				`sources = []`,
				`# effective_source_dirs = []`,
				`# effective_errors:`,
				`# sources = "sources is empty"`,
			},
		},
		{
			name: "blank source",
			toml: `
sources = [" "]
target = "/tmp/out"
`,
			want: []string{
				`sources = [" "]`,
				`# sources = "sources contains an empty source directory"`,
			},
		},
		{
			name: "reserved env profile",
			toml: `
target = "/tmp/out"
profiles = ["work"]
env_profiles = "OVERLAY_TEST_EXTRA_PROFILES"
`,
			env: map[string]string{"OVERLAY_TEST_EXTRA_PROFILES": "base"},
			want: []string{
				`profiles = ["work"]`,
				`# effective_profiles = ["work", "base"]`,
				`# profiles = "profile name \"base\" is reserved`,
			},
		},
		{
			name: "undefined source env var",
			toml: `
sources = ["$OVERLAY_TEST_MISSING_SOURCE"]
target = "/tmp/out"
`,
			want: []string{
				`# effective_source_dirs = ["$OVERLAY_TEST_MISSING_SOURCE"]`,
				`# sources = "expand sources: undefined environment variable(s): OVERLAY_TEST_MISSING_SOURCE"`,
			},
		},
		{
			name: "undefined target env var",
			toml: `
sources = ["pkgs"]
target = "$OVERLAY_TEST_MISSING_TARGET/out"
`,
			want: []string{
				`# effective_target_dir = "$OVERLAY_TEST_MISSING_TARGET/out"`,
				`# target = "expand target: undefined environment variable(s): OVERLAY_TEST_MISSING_TARGET"`,
			},
		},
		{
			name: "invalid ignore pattern",
			toml: `
target = "/tmp/out"
ignore = ["[bad"]
`,
			want: []string{
				`ignore = ["[bad"]`,
				`# ignore = "invalid ignore pattern \"[bad\"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			cfgPath := filepath.Join(dir, ".overlay.toml")
			writeFile(t, cfgPath, tt.toml)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			cmd, g := setupCmd(t, []string{"--config", cfgPath})
			raw, err := loadRawConfig(cmd, g)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := printConfig(&buf, raw); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			wants := append([]string{
				"# provenance",
				`# loaded_files = ["` + cfgPath + `"]`,
			}, tt.want...)
			for _, want := range wants {
				if !strings.Contains(out, want) {
					t.Fatalf("config output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestPrintConfigDoesNotRequireTarget(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd, g := setupCmd(t, nil)
	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := printConfig(&buf, raw); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`target = ""`,
		`# target = "target is required (set in .overlay.toml or pass --target)"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config output missing %q:\n%s", want, out)
		}
	}
}

func TestRunConfigValidateRejectsInvalidIgnorePattern(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
target = "/tmp/out"
ignore = ["[bad"]
`)
	cmd, _ := setupCmd(t, nil)
	err := runConfigValidate(cmd, cfgPath)
	if err == nil {
		t.Fatal("expected invalid ignore pattern error")
	}
	if !strings.Contains(err.Error(), `ignore: invalid ignore pattern "[bad"`) {
		t.Fatalf("error = %v, want invalid ignore pattern", err)
	}
}

func TestRunConfigValidateUsesEnvAndConfigBackedFlags(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = []
`)
	t.Setenv("OVERLAY_TARGET", "/env/out")
	cmd, _ := setupCmd(t, []string{"--source", "flag-src"})
	if err := runConfigValidate(cmd, cfgPath); err != nil {
		t.Fatalf("runConfigValidate() error = %v, want env target and flag source to satisfy validation", err)
	}
}

func TestRunConfigValidateReportsAllEffectiveErrors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
sources = []
ignore = ["[bad"]
`)
	cmd, _ := setupCmd(t, nil)
	err := runConfigValidate(cmd, cfgPath)
	if err == nil {
		t.Fatal("expected effective validation errors")
	}
	for _, want := range []string{"sources: sources is empty", "target: target is required", `ignore: invalid ignore pattern "[bad"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q:\n%v", want, err)
		}
	}
}

func TestResolveVarFlagsLastWins(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	cmd, g := setupCmd(t, []string{"--var", "OVERLAYTEST_A=1", "--var", "OVERLAYTEST_A=2"})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"OVERLAYTEST_A": "2"}
	if !reflect.DeepEqual(r.Effective.Pins, want) {
		t.Errorf("Pins = %v, want %v", r.Effective.Pins, want)
	}
}

func TestResolveVarsCommaSplitAndSingularWins(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	cmd, g := setupCmd(t, []string{"--vars", "OVERLAYTEST_A=1,OVERLAYTEST_B=2", "--var", "OVERLAYTEST_A=3"})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"OVERLAYTEST_A": "3", "OVERLAYTEST_B": "2"}
	if !reflect.DeepEqual(r.Effective.Pins, want) {
		t.Errorf("Pins = %v, want %v", r.Effective.Pins, want)
	}
}

func TestResolveVarsEnvLoadsAndIsReplacedByFlags(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	t.Setenv("OVERLAY_VARS", "OVERLAYTEST_A=env")

	cmd, g := setupCmd(t, nil)
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Effective.Pins, map[string]string{"OVERLAYTEST_A": "env"}) {
		t.Errorf("Pins from env = %v", r.Effective.Pins)
	}

	cmd, g = setupCmd(t, []string{"--var", "OVERLAYTEST_B=flag"})
	r, err = resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Effective.Pins, map[string]string{"OVERLAYTEST_B": "flag"}) {
		t.Errorf("flags should replace OVERLAY_VARS wholesale, got %v", r.Effective.Pins)
	}
}

func TestResolveVarsInTomlRejected(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
vars = ["A=1"]
`)
	cmd, g := setupCmd(t, nil)
	if _, err := resolve(cmd, g); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("vars in TOML should be an unknown-key error, got: %v", err)
	}
}

func TestResolveDeadPinErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	cmd, g := setupCmd(t, []string{"--var", "OTHERPREFIX_A=1"})
	if _, err := resolve(cmd, g); err == nil || !strings.Contains(err.Error(), "substitute_prefixes") {
		t.Fatalf("dead pin should error, got: %v", err)
	}

	writeFile(t, filepath.Join(dir, ".overlay.toml"), "target = \"/tmp/out\"\n")
	cmd, g = setupCmd(t, []string{"--var", "OVERLAYTEST_A=1"})
	if _, err := resolve(cmd, g); err == nil || !strings.Contains(err.Error(), "none configured") {
		t.Fatalf("pin with no prefixes configured should error, got: %v", err)
	}
}

func TestResolveMalformedPinErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	for _, pin := range []string{"NOEQUALS", "1BAD=x"} {
		cmd, g := setupCmd(t, []string{"--var", pin})
		if _, err := resolve(cmd, g); err == nil {
			t.Errorf("pin %q should error", pin)
		}
	}
}

func TestResolvePinsDoNotAffectPathsOrEnvProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "$OVERLAYTEST_TGT"
env_profiles = "OVERLAYTEST_PROFILES"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	t.Setenv("OVERLAYTEST_TGT", "/env/target")
	t.Setenv("OVERLAYTEST_PROFILES", "envprof")
	cmd, g := setupCmd(t, []string{
		"--var", "OVERLAYTEST_TGT=/pin/target",
		"--var", "OVERLAYTEST_PROFILES=pinprof",
	})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if r.Settings.TargetDir != "/env/target" {
		t.Errorf("target should expand from ambient env, not pins: %q", r.Settings.TargetDir)
	}
	if !reflect.DeepEqual(r.Settings.Profiles, []string{"envprof"}) {
		t.Errorf("env_profiles should read ambient env, not pins: %v", r.Settings.Profiles)
	}
}

func TestResolveBuildsSubstituter(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".overlay.toml"), `
target = "/tmp/out"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	t.Setenv("OVERLAYTEST_AMBIENT", "fromenv")
	cmd, g := setupCmd(t, []string{"--var", "OVERLAYTEST_AMBIENT=frompin"})
	r, err := resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Substituter.Enabled() {
		t.Fatal("substituter should be enabled with prefixes configured")
	}
	out, res := r.Substituter.Apply([]byte("${OVERLAYTEST_AMBIENT}"))
	if string(out) != "frompin" {
		t.Errorf("pin should beat ambient env, got %q", out)
	}
	if len(res.Missing) != 0 {
		t.Errorf("unexpected missing: %v", res.Missing)
	}

	writeFile(t, filepath.Join(dir, ".overlay.toml"), "target = \"/tmp/out\"\n")
	cmd, g = setupCmd(t, nil)
	r, err = resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	if r.Substituter.Enabled() {
		t.Error("substituter should be disabled without prefixes")
	}
}

func TestPrintConfigVarsProvenanceWithoutTomlEcho(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, cfgPath, `
target = "/tmp/out"
substitute_prefixes = ["OVERLAYTEST_"]
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath, "--var", "OVERLAYTEST_A=1"})
	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := printConfig(&buf, raw); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertProvenanceRow(t, out, "vars", `["OVERLAYTEST_A=1"]`, pflagloader.SourcePFlag)
	if !strings.Contains(out, `substitute_prefixes = ["OVERLAYTEST_"]`) {
		t.Errorf("substitute_prefixes missing from raw TOML echo:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "vars") {
			t.Fatalf("vars must not appear in the raw TOML echo:\n%s", out)
		}
	}
}
