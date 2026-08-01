package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmcampanini/overlay/internal/state"
)

type rootResult struct {
	code   int
	stdout string
	stderr string
}

func runRoot(t *testing.T, args ...string) rootResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := newRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return rootResult{stdout: stdout.String(), stderr: stderr.String()}
	}
	var code ExitCode
	if !errors.As(err, &code) {
		t.Fatalf("command returned a non-ExitCode error: %v\nstderr:\n%s", err, stderr.String())
	}
	return rootResult{code: int(code), stdout: stdout.String(), stderr: stderr.String()}
}

func orphansFixture(t *testing.T, configBody string) (configPath, target string) {
	t.Helper()
	for _, key := range []string{"OVERLAY_SOURCES", "OVERLAY_TARGET", "OVERLAY_PROFILES"} {
		unsetEnv(t, key)
	}
	dir := t.TempDir()
	configPath = filepath.Join(dir, ".overlay.toml")
	target = filepath.Join(dir, "target")
	writeFile(t, configPath, "target = \""+target+"\"\n"+configBody)
	return configPath, target
}

func TestOrphansCmdMissingBaselineIsExitTwoWithEmptyStdout(t *testing.T) {
	configPath, _ := orphansFixture(t, "")
	result := runRoot(t, "orphans", "--config", configPath)
	if result.code != 2 {
		t.Fatalf("exit = %d, want 2; stderr:\n%s", result.code, result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
	if !strings.Contains(result.stderr, "no state file yet; run `overlay render` to establish the baseline") {
		t.Errorf("stderr missing baseline message:\n%s", result.stderr)
	}
}

func TestOrphansCmdResolutionFailureIsExitTwo(t *testing.T) {
	unsetEnv(t, "OVERLAY_TARGET")
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".overlay.toml")
	writeFile(t, configPath, "sources = [\".\"]\n")
	result := runRoot(t, "orphans", "--config", configPath)
	if result.code != 2 {
		t.Fatalf("exit = %d, want 2", result.code)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
	if !strings.Contains(result.stderr, "overlay:") {
		t.Errorf("stderr missing overlay prefix:\n%s", result.stderr)
	}
}

func TestOrphansCmdInvalidStateIsExitTwoWithEmptyStdout(t *testing.T) {
	configPath, _ := orphansFixture(t, "")
	statePath := filepath.Join(filepath.Dir(configPath), ".overlay.state.json")
	writeFile(t, statePath, "{")

	result := runRoot(t, "orphans", "--config", configPath)
	if result.code != 2 {
		t.Fatalf("exit = %d, want 2; stderr:\n%s", result.code, result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
	if !strings.Contains(result.stderr, "delete it and re-run overlay render") {
		t.Errorf("stderr missing re-baseline remedy:\n%s", result.stderr)
	}
}

func TestOrphansCmdInspectionFailureIsExitTwoWithEmptyStdout(t *testing.T) {
	configPath, _ := orphansFixture(t, "")
	invalidTarget := filepath.Join(filepath.Dir(configPath), "invalid\x00target")
	manifest, err := json.Marshal(struct {
		Entries []state.Entry `json:"entries"`
	}{Entries: []state.Entry{{Target: invalidTarget, Source: filepath.Dir(configPath)}}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(filepath.Dir(configPath), ".overlay.state.json"), string(manifest))

	result := runRoot(t, "orphans", "--config", configPath)
	if result.code != 2 {
		t.Fatalf("exit = %d, want 2; stderr:\n%s", result.code, result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
	for _, want := range []string{"detect orphans", "inspect owned target"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, result.stderr)
		}
	}
}

func TestOrphansCmdRenderThenMutatePrintsExactSortedTargets(t *testing.T) {
	configPath, target := orphansFixture(t, "sources = [\"one\", \"two\"]\nprofiles = [\"work\"]\n")
	rootDir := filepath.Dir(configPath)
	one := filepath.Join(rootDir, "one")
	two := filepath.Join(rootDir, "two")
	writeFile(t, filepath.Join(one, "z.olay.base.conf"), "z\n")
	writeFile(t, filepath.Join(two, "b.olay.base.conf"), "b\n")
	writeFile(t, filepath.Join(two, "a.olay.work.conf"), "a\n")

	if result := runRoot(t, "render", "--config", configPath); result.code != 0 {
		t.Fatalf("render exit = %d, want 0; stderr:\n%s", result.code, result.stderr)
	}
	clean := runRoot(t, "orphans", "--config", configPath)
	if clean.code != 0 || clean.stdout != "" {
		t.Fatalf("clean orphans = exit %d stdout %q, want exit 0 and empty stdout; stderr:\n%s", clean.code, clean.stdout, clean.stderr)
	}

	writeFile(t, configPath, "target = \""+target+"\"\nsources = [\"one\", \"two\"]\n")
	if err := os.RemoveAll(one); err != nil {
		t.Fatal(err)
	}

	result := runRoot(t, "orphans", "--config", configPath)
	if result.code != 1 {
		t.Fatalf("exit = %d, want 1; stderr:\n%s", result.code, result.stderr)
	}
	want := filepath.Join(target, "a.conf") + "\n" + filepath.Join(target, "z.conf") + "\n"
	if result.stdout != want {
		t.Errorf("stdout = %q, want %q", result.stdout, want)
	}

	narrowed := runRoot(t, "orphans", "--config", configPath, "two")
	if narrowed.code != 1 {
		t.Fatalf("narrowed exit = %d, want 1; stderr:\n%s", narrowed.code, narrowed.stderr)
	}
	if want := filepath.Join(target, "a.conf") + "\n"; narrowed.stdout != want {
		t.Errorf("narrowed stdout = %q, want %q", narrowed.stdout, want)
	}

	writeFile(t, configPath, "target = \""+target+"\"\nsources = [\"two\"]\nprofiles = [\"work\"]\n")
	removedSource := runRoot(t, "orphans", "--config", configPath)
	if removedSource.code != 1 || removedSource.stdout != filepath.Join(target, "z.conf")+"\n" {
		t.Fatalf("removed source result = exit %d stdout %q; stderr:\n%s", removedSource.code, removedSource.stdout, removedSource.stderr)
	}
	explicitRemoved := runRoot(t, "orphans", "--config", configPath, "one")
	if explicitRemoved.code != 1 || explicitRemoved.stdout != filepath.Join(target, "z.conf")+"\n" {
		t.Fatalf("explicit removed source result = exit %d stdout %q; stderr:\n%s", explicitRemoved.code, explicitRemoved.stdout, explicitRemoved.stderr)
	}
	selectedActive := runRoot(t, "orphans", "--config", configPath, "two")
	if selectedActive.code != 0 || selectedActive.stdout != "" {
		t.Fatalf("active source result = exit %d stdout %q; stderr:\n%s", selectedActive.code, selectedActive.stdout, selectedActive.stderr)
	}
}

func TestOrphansCmdRenameReportsOnlyOldTarget(t *testing.T) {
	configPath, target := orphansFixture(t, "")
	sourceDir := filepath.Dir(configPath)
	oldLayer := filepath.Join(sourceDir, "old.olay.base.conf")
	newLayer := filepath.Join(sourceDir, "new.olay.base.conf")
	writeFile(t, oldLayer, "value\n")
	if result := runRoot(t, "render", "--config", configPath); result.code != 0 {
		t.Fatalf("render exit = %d; stderr:\n%s", result.code, result.stderr)
	}
	if err := os.Rename(oldLayer, newLayer); err != nil {
		t.Fatal(err)
	}

	result := runRoot(t, "orphans", "--config", configPath)
	want := filepath.Join(target, "old.conf") + "\n"
	if result.code != 1 || result.stdout != want {
		t.Fatalf("rename result = exit %d stdout %q, want exit 1 stdout %q; stderr:\n%s", result.code, result.stdout, want, result.stderr)
	}
}

func TestOrphansCmdTargetChangeReportsOldRoot(t *testing.T) {
	configPath, oldTarget := orphansFixture(t, "")
	writeFile(t, filepath.Join(filepath.Dir(configPath), "x.olay.base.conf"), "value\n")
	if result := runRoot(t, "render", "--config", configPath); result.code != 0 {
		t.Fatalf("render exit = %d; stderr:\n%s", result.code, result.stderr)
	}
	newTarget := filepath.Join(filepath.Dir(configPath), "new-target")
	writeFile(t, configPath, "target = \""+newTarget+"\"\n")

	result := runRoot(t, "orphans", "--config", configPath)
	want := filepath.Join(oldTarget, "x.conf") + "\n"
	if result.code != 1 || result.stdout != want {
		t.Fatalf("target-change result = exit %d stdout %q, want exit 1 stdout %q; stderr:\n%s", result.code, result.stdout, want, result.stderr)
	}
}

func TestOrphansCmdConfigsSharingTargetKeepIndependentState(t *testing.T) {
	for _, key := range []string{"OVERLAY_SOURCES", "OVERLAY_TARGET", "OVERLAY_PROFILES"} {
		unsetEnv(t, key)
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	configA := filepath.Join(root, "a", ".overlay.toml")
	configB := filepath.Join(root, "b", ".overlay.toml")
	writeFile(t, configA, "target = \""+target+"\"\n")
	writeFile(t, configB, "target = \""+target+"\"\n")
	layerA := filepath.Join(filepath.Dir(configA), "a.olay.base.conf")
	writeFile(t, layerA, "a\n")
	writeFile(t, filepath.Join(filepath.Dir(configB), "b.olay.base.conf"), "b\n")

	for _, configPath := range []string{configA, configB} {
		if result := runRoot(t, "render", "--config", configPath); result.code != 0 {
			t.Fatalf("render %s exit = %d; stderr:\n%s", configPath, result.code, result.stderr)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(configPath), ".overlay.state.json")); err != nil {
			t.Fatalf("state for %s: %v", configPath, err)
		}
	}
	if err := os.Remove(layerA); err != nil {
		t.Fatal(err)
	}

	resultA := runRoot(t, "orphans", "--config", configA)
	wantA := filepath.Join(target, "a.conf") + "\n"
	if resultA.code != 1 || resultA.stdout != wantA {
		t.Fatalf("config A result = exit %d stdout %q; stderr:\n%s", resultA.code, resultA.stdout, resultA.stderr)
	}
	resultB := runRoot(t, "orphans", "--config", configB)
	if resultB.code != 0 || resultB.stdout != "" {
		t.Fatalf("config B result = exit %d stdout %q; stderr:\n%s", resultB.code, resultB.stdout, resultB.stderr)
	}
}
