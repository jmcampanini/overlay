// Package gittest builds git fixtures for tests that run the real git binary.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// NewBareRepo creates a bare repository with one commit on main and returns
// its path. It also isolates git from the developer's global and system
// configuration for the rest of the test. The test fails when git is absent.
func NewBareRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	dir := t.TempDir()
	bare := filepath.Join(dir, "fixture.git")
	work := filepath.Join(dir, "work")
	Run(t, dir, "init", "-q", "--bare", bare)
	Run(t, dir, "init", "-q", "-b", "main", work)
	Run(t, work, "-c", "user.name=fixture", "-c", "user.email=fixture@example.com",
		"commit", "-q", "--allow-empty", "-m", "fixture")
	Run(t, work, "push", "-q", bare, "main")
	return bare
}

// Run executes git with args in dir and fails the test on any error.
func Run(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}
