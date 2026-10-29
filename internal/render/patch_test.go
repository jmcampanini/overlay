package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jmcampanini/overlay/internal/config"
)

func TestWritePatchDescribesNewAndChangedFilesRelativeToTheTarget(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/dot-config/new", "new\n", 0o600)
	src.file("pkg/empty", "", 0o644)
	src.file("pkg/exec", "#!/bin/sh\n", 0o755)
	src.file("pkg/same", "same\n", 0o644)
	src.file("pkg/content", "one\ntwo\nthree\n", 0o644)
	src.file("pkg/noeol", "y", 0o644)
	src.file("pkg/mode", "mode\n", 0o600)
	src.file("pkg/link", "linked\n", 0o644)
	src.file("pkg/greeting.tmpl", "hello {{ .OS }}\n", 0o644)
	src.file(`pkg/odd "name"`, "quoted\n", 0o644)
	src.file("pkg/with space", "spaced\n", 0o644)
	req := templateRequest(t, src)
	req.OS = "plan9"
	target := req.Target
	writeFile(t, filepath.Join(target, "same"), "same\n", 0o644)
	writeFile(t, filepath.Join(target, "content"), "one\n2\nthree\n", 0o644)
	writeFile(t, filepath.Join(target, "noeol"), "x\n", 0o644)
	writeFile(t, filepath.Join(target, "mode"), "mode\n", 0o644)
	writeFile(t, filepath.Join(target, "greeting"), "hello there\n", 0o644)
	writeFile(t, filepath.Join(target, "link.target"), "linked\n", 0o644)
	if err := os.Symlink("link.target", filepath.Join(target, "link")); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var out bytes.Buffer
	omissions, err := WritePatch(&out, plan)
	if err != nil {
		t.Fatalf("WritePatch() error = %v", err)
	}

	want := `diff --git a/.config/new b/.config/new
new file mode 100644
--- /dev/null
+++ b/.config/new
@@ -0,0 +1 @@
+new
diff --git a/content b/content
--- a/content
+++ b/content
@@ -1,3 +1,3 @@
 one
-2
+two
 three
diff --git a/empty b/empty
new file mode 100644
diff --git a/exec b/exec
new file mode 100755
--- /dev/null
+++ b/exec
@@ -0,0 +1 @@
+#!/bin/sh
diff --git a/greeting b/greeting
--- a/greeting
+++ b/greeting
@@ -1 +1 @@
-hello there
+hello plan9
diff --git a/link b/link
deleted file mode 120000
--- a/link
+++ /dev/null
@@ -1 +0,0 @@
-link.target
\ No newline at end of file
diff --git a/link b/link
new file mode 100644
--- /dev/null
+++ b/link
@@ -0,0 +1 @@
+linked
diff --git a/noeol b/noeol
--- a/noeol
+++ b/noeol
@@ -1 +1 @@
-x
+y
\ No newline at end of file
diff --git "a/odd \"name\"" "b/odd \"name\""
new file mode 100644
--- /dev/null
+++ "b/odd \"name\""
@@ -0,0 +1 @@
+quoted
diff --git "a/with space" "b/with space"
new file mode 100644
--- /dev/null
+++ "b/with space"
@@ -0,0 +1 @@
+spaced
`
	if diff := cmp.Diff(want, out.String()); diff != "" {
		t.Errorf("patch mismatch (-want +got):\n%s", diff)
	}
	gotOmissions := map[string]string{}
	for _, omission := range omissions {
		gotOmissions[filepath.Base(omission.Entry.Target)] = omission.Reason
	}
	wantOmissions := map[string]string{"mode": "set permission bits 644 -> 600"}
	if diff := cmp.Diff(wantOmissions, gotOmissions); diff != "" {
		t.Errorf("omissions mismatch (-want +got):\n%s", diff)
	}
}

func TestWritePatchIsEmptyWhenNothingChanges(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/same", "same\n", 0o644)
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "same"), "same\n", 0o644)
	plan, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var out bytes.Buffer
	omissions, err := WritePatch(&out, plan)

	if err != nil || out.Len() != 0 || len(omissions) != 0 {
		t.Errorf("WritePatch() = %q, %v, %v; want empty output, no omissions, nil error", out.String(), omissions, err)
	}
}

func TestWritePatchRejectsATargetThatIsNotAFile(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/first", "first\n", 0o644)
	src.file("pkg/pipe", "pipe\n", 0o644)
	target := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(target, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var out bytes.Buffer
	_, err = WritePatch(&out, plan)

	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("WritePatch() error = %v, want one naming the non-regular target", err)
	}
	if !strings.HasPrefix(out.String(), "diff --git a/first b/first\n") {
		t.Errorf("the patch for entries before the failure is missing:\n%s", out.String())
	}
}

func TestQuotePath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "a/.bashrc", want: "a/.bashrc"},
		{path: "a/café", want: "a/café"},
		{path: "a/with space", want: `"a/with space"`},
		{path: `a/say "hi"`, want: `"a/say \"hi\""`},
		{path: `a/back\slash`, want: `"a/back\\slash"`},
		{path: "a/tab\there", want: `"a/tab\there"`},
		{path: "a/new\nline", want: `"a/new\nline"`},
		{path: "a/bell\x07", want: `"a/bell\007"`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := quotePath(tt.path); got != tt.want {
				t.Errorf("quotePath(%q) = %s, want %s", tt.path, got, tt.want)
			}
		})
	}
}

func TestWritePatchPrintsMergedContent(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n[layers.local]\n")
	src.file("pkg/settings.olay.base.json", `{"a": 1}`, 0o644)
	src.file("pkg/settings.olay.local.json", `{"b": "${OVERLAY_TEST_PATCH}"}`, 0o644)
	t.Setenv("OVERLAY_TEST_PATCH", "two")
	target := t.TempDir()
	plan, err := Build(Request{Layers: []string{"base", "local"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var out bytes.Buffer
	if _, err := WritePatch(&out, plan); err != nil {
		t.Fatalf("WritePatch() error = %v", err)
	}

	want := "+++ b/settings.json\n@@ -0,0 +1,4 @@\n+{\n+  \"a\": 1,\n+  \"b\": \"two\"\n+}\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("patch lacks the merged document %q:\n%s", want, out.String())
	}
	if strings.Contains(out.String(), `{"a": 1}`) {
		t.Errorf("patch shows raw fragment bytes:\n%s", out.String())
	}
}
