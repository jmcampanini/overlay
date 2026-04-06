package cli

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestResolveDefaultEnvFallback(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv(DefaultEnvProfiles, "auto1,auto2")
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
source = "pkgs"
target = "/tmp/out"
`)
	cmd, g := setupCmd(t, []string{"--config", cfgPath})
	r, err := Resolve(cmd, g)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(sub, "pkgs")
	if r.Settings.SourceDir != want {
		t.Errorf("SourceDir = %q, want %q", r.Settings.SourceDir, want)
	}
	if r.Provenance.Source != ProvConfig {
		t.Errorf("SourceFrom = %v, want config", r.Provenance.Source)
	}
}

func TestResolveSourceFlagOverride(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".overlay.toml")
	writeFile(t, cfgPath, `
source = "from-config"
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
	if r.Settings.SourceDir != "/absolute/override" {
		t.Errorf("SourceDir = %q, want /absolute/override", r.Settings.SourceDir)
	}
	if r.Provenance.Source != ProvFlag {
		t.Errorf("SourceFrom = %v, want flag", r.Provenance.Source)
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
	if r.Settings.SourceDir != home {
		t.Errorf("SourceDir = %q, want %q (home)", r.Settings.SourceDir, home)
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
	if r.Settings.SourceDir != "/custom/src" {
		t.Errorf("SourceDir = %q, want /custom/src", r.Settings.SourceDir)
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
