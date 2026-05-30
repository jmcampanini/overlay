package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustGlob(t *testing.T, patterns []string) Ignorer {
	t.Helper()
	ign, err := NewGlobIgnorer(patterns)
	if err != nil {
		t.Fatalf("NewGlobIgnorer: %v", err)
	}
	return ign
}

func TestGlobIgnorerMatchesBasename(t *testing.T) {
	ign := mustGlob(t, []string{"node_modules"})
	if !ign.Match("a/b/node_modules", true) {
		t.Error("should match path ending in node_modules")
	}
	if !ign.Match("node_modules", true) {
		t.Error("should match bare node_modules")
	}
	if ign.Match("a/b/src", false) {
		t.Error("should not match src")
	}
}

func TestGlobIgnorerTrailingSlashDirOnly(t *testing.T) {
	ign := mustGlob(t, []string{"vendor/"})
	if !ign.Match("vendor", true) {
		t.Error("vendor/ should match vendor as a directory")
	}
	if !ign.Match("a/b/vendor", true) {
		t.Error("vendor/ should match a/b/vendor as a directory")
	}
	if ign.Match("vendor", false) {
		t.Error("vendor/ should not match vendor as a file")
	}
}

func TestGlobIgnorerDoublestar(t *testing.T) {
	ign := mustGlob(t, []string{"**/vendor"})
	if !ign.Match("a/b/c/vendor", true) {
		t.Error("**/vendor should match deep path")
	}
}

func TestGlobIgnorerEmptyPatternsIgnored(t *testing.T) {
	ign := mustGlob(t, []string{"", "  "})
	if ign.Match("anything", false) {
		t.Error("empty patterns should not match")
	}
}

func TestGlobIgnorerInvalidPatternErrors(t *testing.T) {
	_, err := NewGlobIgnorer([]string{"[unclosed"})
	if err == nil {
		t.Fatal("expected error for invalid glob pattern")
	}
	if !strings.Contains(err.Error(), "invalid ignore pattern") {
		t.Errorf("error should mention 'invalid ignore pattern': %v", err)
	}
}

func TestNormalizeGlobPatterns(t *testing.T) {
	got, err := NormalizeGlobPatterns([]string{"", " node_modules ", "vendor/"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"node_modules", "vendor/"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("NormalizeGlobPatterns = %v, want %v", got, want)
	}
}

func TestValidateGlobPatternsInvalidPatternErrors(t *testing.T) {
	err := ValidateGlobPatterns([]string{"[unclosed"})
	if err == nil {
		t.Fatal("expected error for invalid glob pattern")
	}
	if !strings.Contains(err.Error(), "invalid ignore pattern") {
		t.Errorf("error should mention 'invalid ignore pattern': %v", err)
	}
}

func TestNoopIgnorer(t *testing.T) {
	ign := NoopIgnorer()
	if ign.Match("anything", true) {
		t.Error("noop should never match")
	}
}

func TestChainIgnorer(t *testing.T) {
	a := mustGlob(t, []string{"a"})
	b := mustGlob(t, []string{"b"})
	ch := NewChain(a, b, nil)
	if !ch.Match("a", false) {
		t.Error("should match via first ignorer")
	}
	if !ch.Match("b", false) {
		t.Error("should match via second ignorer")
	}
	if ch.Match("c", false) {
		t.Error("should not match c")
	}
}

func TestGitignoreIgnorerMissingFile(t *testing.T) {
	ign, err := NewGitignoreIgnorer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ign.Match("anything", false) {
		t.Error("missing .gitignore should never match")
	}
}

func TestGitignoreIgnorerReal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ign, err := NewGitignoreIgnorer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ign.Match("foo.log", false) {
		t.Error("should match *.log")
	}
	if !ign.Match("build/x", false) {
		t.Error("should match build/")
	}
	if ign.Match("main.go", false) {
		t.Error("should not match main.go")
	}
}
