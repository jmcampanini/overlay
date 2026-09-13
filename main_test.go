package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRejectedInputLeavesRenderedStateUnchanged runs the built binary against
// the basic-json fixture. After a valid render, input that Cobra rejects (an
// unknown flag, an unexpected operand) must leave the target tree and the
// state file exactly as the render left them.
func TestRejectedInputLeavesRenderedStateUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the overlay binary")
	}
	binary := buildOverlay(t)
	dir := copyFixture(t, filepath.Join("testdata", "fixtures", "basic-json"))
	configPath := filepath.Join(dir, ".overlay.toml")

	render := runOverlay(t, binary, "render", "--config", configPath)
	if render.code != 0 {
		t.Fatalf("render exit = %d, want 0; stderr:\n%s", render.code, render.stderr)
	}
	before := snapshotTree(t, dir)
	rendered := before[filepath.Join(".generated", ".claude", "settings.json")].content
	expected := readFile(t, filepath.Join("testdata", "fixtures", "basic-json", "expected", ".claude", "settings.json"))
	if rendered != expected {
		t.Fatalf("rendered settings.json = %q, want fixture expected %q", rendered, expected)
	}
	if _, ok := before[".overlay.state.json"]; !ok {
		t.Fatalf("render did not write .overlay.state.json; tree: %v", before)
	}

	for _, rejected := range [][]string{
		{"render", "--bogus"},
		{"config", "extra"},
	} {
		result := runOverlay(t, binary, append(rejected, "--config", configPath)...)
		if result.code == 0 {
			t.Errorf("%v exit = 0, want nonzero; stdout:\n%s", rejected, result.stdout)
		}
		after := snapshotTree(t, dir)
		if len(after) != len(before) {
			t.Errorf("%v changed the file set: before %d files, after %d", rejected, len(before), len(after))
		}
		for path, want := range before {
			if got := after[path]; got != want {
				t.Errorf("%v changed %s: modtime %s -> %s", rejected, path, want.modTime, got.modTime)
			}
		}
	}
}

type overlayResult struct {
	code   int
	stdout string
	stderr string
}

func runOverlay(t *testing.T, binary string, args ...string) overlayResult {
	t.Helper()
	command := exec.Command(binary, args...)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "OVERLAY_") {
			command.Env = append(command.Env, entry)
		}
	}

	err := command.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return overlayResult{stdout: stdout.String(), stderr: stderr.String()}
	case errors.As(err, &exit):
		return overlayResult{code: exit.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
	default:
		t.Fatalf("run %s %v: %v", binary, args, err)
		return overlayResult{}
	}
}

func buildOverlay(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "overlay")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}
	return binary
}

// copyFixture copies the fixture's config file and source tree into a fresh
// temporary directory so the render can write beside them.
func copyFixture(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(filepath.Join(dir, "source"), os.DirFS(filepath.Join(fixture, "source"))); err != nil {
		t.Fatalf("copy fixture source: %v", err)
	}
	config := readFile(t, filepath.Join(fixture, ".overlay.toml"))
	if err := os.WriteFile(filepath.Join(dir, ".overlay.toml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	return dir
}

type fileState struct {
	content string
	modTime string
}

// snapshotTree records every regular file under dir by relative path with its
// content and modification time, so a rewrite of identical bytes still shows.
func snapshotTree(t *testing.T, dir string) map[string]fileState {
	t.Helper()
	files := map[string]fileState{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[relative] = fileState{content: readFile(t, path), modTime: info.ModTime().String()}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return files
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
