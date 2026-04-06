package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetPathDotPrefix(t *testing.T) {
	home, _ := os.UserHomeDir()
	got, err := TargetPath("dot-claude", "settings", "json", "~/", true)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	want := filepath.Join(home, ".claude", "settings.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetPathNestedDotPrefix(t *testing.T) {
	home, _ := os.UserHomeDir()
	got, err := TargetPath("dot-config/starship", "extra", "toml", "~/", true)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	want := filepath.Join(home, ".config", "starship", "extra.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetPathNoDotPrefix(t *testing.T) {
	got, err := TargetPath("dot-claude", "settings", "json", "/tmp/out", false)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	want := filepath.Join("/tmp/out", "dot-claude", "settings.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetPathEnvVarExpansion(t *testing.T) {
	t.Setenv("OVERLAY_TEST_TARGET", "/tmp/expanded")
	got, err := TargetPath("sub", "file", "json", "$OVERLAY_TEST_TARGET/out", false)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	if !strings.Contains(got, "/tmp/expanded/out/sub/file.json") {
		t.Errorf("env var not expanded: %q", got)
	}
}

func TestTargetPathEmptyRelDir(t *testing.T) {
	got, err := TargetPath("", "root", "json", "/tmp", true)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	if got != "/tmp/root.json" {
		t.Errorf("got %q", got)
	}
}

func TestTargetPathEmptyTargetErrors(t *testing.T) {
	if _, err := TargetPath("x", "y", "json", "", true); err == nil {
		t.Error("expected error for empty target")
	}
}

func TestTargetPathDotPrefixedStemAtRoot(t *testing.T) {
	got, err := TargetPath("", "dot-settings", "json", "/tmp/out", true)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	want := filepath.Join("/tmp/out", ".settings.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetPathDotPrefixedStemInSubdir(t *testing.T) {
	got, err := TargetPath("pkg", "dot-rc", "toml", "/tmp/out", true)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	want := filepath.Join("/tmp/out", "pkg", ".rc.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetPathDotPrefixOffLeavesStemAlone(t *testing.T) {
	got, err := TargetPath("", "dot-settings", "json", "/tmp/out", false)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	want := filepath.Join("/tmp/out", "dot-settings.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandPathTildeOnly(t *testing.T) {
	home, _ := os.UserHomeDir()
	got, err := ExpandPath("~")
	if err != nil {
		t.Fatal(err)
	}
	if got != home {
		t.Errorf("got %q, want %q", got, home)
	}
}

func TestExpandPathEmpty(t *testing.T) {
	got, err := ExpandPath("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
