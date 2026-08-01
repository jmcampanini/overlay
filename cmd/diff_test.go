package cmd

import (
	"errors"
	"path/filepath"
	"testing"
)

// runDiff executes the real diff command and returns its exit code: 0 when
// RunE returns nil, otherwise the wrapped ExitCode value.
func runDiff(t *testing.T, args ...string) int {
	t.Helper()
	root := newRootCmd()
	root.SetArgs(append([]string{"diff"}, args...))
	err := root.Execute()
	if err == nil {
		return 0
	}
	var ec ExitCode
	if errors.As(err, &ec) {
		return int(ec)
	}
	t.Fatalf("diff returned a non-ExitCode error: %v", err)
	return -1
}

func diffFixture(t *testing.T, configBody string) (cfgPath, target string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "src", ".overlay.toml")
	target = filepath.Join(dir, "target")
	writeFile(t, cfgPath, "target = \""+target+"\"\n"+configBody)
	return cfgPath, target
}

func TestDiffCmdExitCodeClean(t *testing.T) {
	cfg, target := diffFixture(t, "")
	writeFile(t, filepath.Join(filepath.Dir(cfg), "x.olay.base.conf"), "hello\n")
	writeFile(t, filepath.Join(target, "x.conf"), "hello\n") // matches rendered output
	if got := runDiff(t, "--config", cfg); got != 0 {
		t.Errorf("clean diff exit = %d, want 0", got)
	}
}

func TestDiffCmdExitCodeDrift(t *testing.T) {
	cfg, target := diffFixture(t, "")
	writeFile(t, filepath.Join(filepath.Dir(cfg), "x.olay.base.conf"), "hello\n")
	writeFile(t, filepath.Join(target, "x.conf"), "different\n")
	if got := runDiff(t, "--config", cfg); got != 1 {
		t.Errorf("drift diff exit = %d, want 1", got)
	}
}

func TestDiffCmdExitCodeMissingVar(t *testing.T) {
	cfg, _ := diffFixture(t, "substitute_prefixes = [\"PRE_\"]\n")
	writeFile(t, filepath.Join(filepath.Dir(cfg), "y.olay.base.conf"), "v=${PRE_MISSING}\n")
	if got := runDiff(t, "--config", cfg); got != 2 {
		t.Errorf("missing-var diff exit = %d, want 2", got)
	}
}
