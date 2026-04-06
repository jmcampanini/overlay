package diff

import (
	"strings"
	"testing"
)

func TestUnifiedIdentical(t *testing.T) {
	out := Unified([]byte("a\nb\nc\n"), []byte("a\nb\nc\n"), "a", "b")
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestUnifiedAllNew(t *testing.T) {
	out := Unified(nil, []byte("a\nb\n"), "a/f", "b/f")
	if !strings.Contains(out, "--- a/f") {
		t.Errorf("missing --- header:\n%s", out)
	}
	if !strings.Contains(out, "+++ b/f") {
		t.Errorf("missing +++ header:\n%s", out)
	}
	if !strings.Contains(out, "+a") || !strings.Contains(out, "+b") {
		t.Errorf("missing additions:\n%s", out)
	}
	// POSIX unified diff: empty "from" side must use 0 as the start offset.
	if !strings.Contains(out, "@@ -0,0 +1,") {
		t.Errorf("expected header `@@ -0,0 +1,...`, got:\n%s", out)
	}
}

func TestUnifiedAllDeleted(t *testing.T) {
	out := Unified([]byte("a\nb\n"), nil, "a/f", "b/f")
	// POSIX unified diff: empty "to" side must use 0 as the start offset.
	if !strings.Contains(out, "@@ -1,2 +0,0 @@") {
		t.Errorf("expected header `@@ -1,2 +0,0 @@`, got:\n%s", out)
	}
}

func TestUnifiedTrailingContextClampedAtEOF(t *testing.T) {
	// 10 lines; change the first one. With context=3, the hunk header
	// should be @@ -1,4 +1,4 @@ and only 3 trailing context lines
	// should appear (not the whole tail of the file).
	a := []byte("zero\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	b := []byte("ZERO\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	out := Unified(a, b, "a", "b")
	if !strings.Contains(out, "@@ -1,4 +1,4 @@") {
		t.Errorf("expected trailing context clamped to 3 lines, got:\n%s", out)
	}
	// The hunk should not include lines five/six/.../nine as context.
	if strings.Contains(out, "five") || strings.Contains(out, "nine") {
		t.Errorf("trailing context leaked beyond 3 lines:\n%s", out)
	}
}

func TestUnifiedLeadingContextClampedAtBOF(t *testing.T) {
	// Change the last line. Leading context should still clamp to 3.
	a := []byte("zero\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	b := []byte("zero\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n")
	out := Unified(a, b, "a", "b")
	if !strings.Contains(out, "@@ -7,4 +7,4 @@") {
		t.Errorf("expected leading context clamped to 3 lines, got:\n%s", out)
	}
	if strings.Contains(out, "zero") || strings.Contains(out, "five") {
		t.Errorf("leading context leaked before 3 lines:\n%s", out)
	}
}

func TestUnifiedOneLineChange(t *testing.T) {
	a := []byte("one\ntwo\nthree\n")
	b := []byte("one\ntwoX\nthree\n")
	out := Unified(a, b, "a", "b")
	if !strings.Contains(out, "-two") {
		t.Errorf("missing deletion:\n%s", out)
	}
	if !strings.Contains(out, "+twoX") {
		t.Errorf("missing insertion:\n%s", out)
	}
	if !strings.Contains(out, " one") || !strings.Contains(out, " three") {
		t.Errorf("missing context lines:\n%s", out)
	}
}

func TestUnifiedHunkHeader(t *testing.T) {
	a := []byte("one\ntwo\nthree\n")
	b := []byte("one\ntwoX\nthree\n")
	out := Unified(a, b, "a", "b")
	if !strings.Contains(out, "@@ -") || !strings.Contains(out, "+") {
		t.Errorf("missing hunk header:\n%s", out)
	}
}

func TestUnifiedEmptyInputs(t *testing.T) {
	out := Unified(nil, nil, "a", "b")
	if out != "" {
		t.Errorf("expected empty diff, got %q", out)
	}
}

func TestSplitLines(t *testing.T) {
	lines := splitLines([]byte("a\nb\nc\n"))
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "a\n" || lines[1] != "b\n" || lines[2] != "c\n" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestSplitLinesNoTrailingNewline(t *testing.T) {
	lines := splitLines([]byte("a\nb"))
	if len(lines) != 2 || lines[1] != "b" {
		t.Errorf("unexpected lines: %v", lines)
	}
}
