package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderNoStateFlagIsRenderScoped(t *testing.T) {
	root := newRootCmd()
	for commandName, want := range map[string]bool{
		"render":  true,
		"diff":    false,
		"orphans": false,
		"plan":    false,
		"config":  false,
	} {
		command, _, err := root.Find([]string{commandName})
		if err != nil {
			t.Fatalf("find %s: %v", commandName, err)
		}
		if got := command.Flags().Lookup("no-state") != nil; got != want {
			t.Errorf("%s has --no-state = %t, want %t", commandName, got, want)
		}
	}
}

func TestRenderCmdRejectsInactiveYAMLExtensionConflict(t *testing.T) {
	for _, key := range []string{"OVERLAY_SOURCES", "OVERLAY_TARGET", "OVERLAY_PROFILES"} {
		unsetEnv(t, key)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".overlay.toml")
	target := filepath.Join(dir, "target")
	writeFile(t, configPath, "sources = [\"source\"]\ntarget = \""+target+"\"\n")
	writeFile(t, filepath.Join(dir, "source", "config.olay.base.yaml"), "base: true\n")
	writeFile(t, filepath.Join(dir, "source", "config.olay.work.yml"), "work: true\n")

	result := runRoot(t, "render", "--config", configPath)
	if result.code == 0 {
		t.Fatalf("render exit = 0, want nonzero; stderr:\n%s", result.stderr)
	}
	for _, want := range []string{"extension", ".yaml", ".yml", "conflict"} {
		if !strings.Contains(strings.ToLower(result.stderr), want) {
			t.Errorf("stderr missing %q:\n%s", want, result.stderr)
		}
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("render must not write target on extension conflict, stat error = %v", err)
	}
}

func TestRenderCmdNoStateWritesTargetWithoutCreatingSidecar(t *testing.T) {
	for _, key := range []string{"OVERLAY_SOURCES", "OVERLAY_TARGET", "OVERLAY_PROFILES"} {
		unsetEnv(t, key)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".overlay.toml")
	target := filepath.Join(dir, "target")
	writeFile(t, configPath, "sources = [\"source\"]\ntarget = \""+target+"\"\n")
	writeFile(t, filepath.Join(dir, "source", "disposable.olay.base.conf"), "rendered\n")

	result := runRoot(t, "render", "--no-state", "--config", configPath)
	if result.code != 0 {
		t.Fatalf("render exit = %d, want 0; stderr:\n%s", result.code, result.stderr)
	}
	contents, err := os.ReadFile(filepath.Join(target, "disposable.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "rendered\n" {
		t.Errorf("target = %q, want rendered content", contents)
	}
	if _, err := os.Stat(filepath.Join(dir, ".overlay.state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state sidecar must remain absent, stat error = %v", err)
	}
}
