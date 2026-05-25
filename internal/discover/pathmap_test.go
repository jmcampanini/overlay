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

func TestTargetPathExtensionless(t *testing.T) {
	got, err := TargetPath("docs", "README", "", "/tmp/out", true)
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	want := filepath.Join("/tmp/out", "docs", "README")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetPathRejectsDotSegmentExtensionlessName(t *testing.T) {
	cases := []string{".", "..", "dot-", "dot-."}
	for _, stem := range cases {
		_, err := TargetPath("", stem, "", "/tmp/out", true)
		if err == nil {
			t.Fatalf("TargetPath with stem %q should error", stem)
		}
		if !strings.Contains(err.Error(), "invalid target filename") {
			t.Fatalf("TargetPath with stem %q error = %v", stem, err)
		}
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

func TestExpandPathUndefinedEnvVarErrors(t *testing.T) {
	_, err := ExpandPath("$DEFINITELY_NOT_SET_OVERLAY_TEST/x")
	if err == nil {
		t.Fatal("expected error for undefined env var")
	}
	if !strings.Contains(err.Error(), "DEFINITELY_NOT_SET_OVERLAY_TEST") {
		t.Errorf("error should name the missing var: %v", err)
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("error should mention 'undefined': %v", err)
	}
}

func TestExpandPathReportsAllMissing(t *testing.T) {
	_, err := ExpandPath("$OVL_MISS_A/$OVL_MISS_B")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OVL_MISS_A") || !strings.Contains(err.Error(), "OVL_MISS_B") {
		t.Errorf("error should name both missing vars: %v", err)
	}
}

func TestExpandPathPartiallyDefinedErrors(t *testing.T) {
	t.Setenv("OVL_DEF_A", "/x")
	_, err := ExpandPath("$OVL_DEF_A/$OVL_MISS_C")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OVL_MISS_C") {
		t.Errorf("error should name the missing var: %v", err)
	}
	if strings.Contains(err.Error(), "OVL_DEF_A") {
		t.Errorf("error should not mention defined var: %v", err)
	}
}
