package discover

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmcampanini/overlay/internal/document"
)

func TestParseOverlayName(t *testing.T) {
	cases := []struct {
		name               string
		stem, profile, ext string
		ok                 bool
	}{
		{"settings.olay.base.json", "settings", "base", "json", true},
		{"settings.olay.work.json", "settings", "work", "json", true},
		{"config.olay.local.toml", "config", "local", "toml", true},
		{"multi.dot.stem.olay.base.json", "multi.dot.stem", "base", "json", true},
		{"settings.schema.olay.work.json", "settings.schema", "work", "json", true},
		{"foo.olay.olay.json", "foo", "olay", "json", true},
		{"file.olay.base.yaml", "file", "base", "yaml", true},
		{"README.olay.local", "README", "local", "", true},
		{"no-marker.json", "", "", "", false},
		{"archive.olay.work.tar.gz", "", "", "", false},
		{"olay.base.json", "", "", "", false},
		{".olay.base.json", "", "", "", false},
		{"script.olay..sh", "", "", "", false},
		{"stem.notolay.profile.json", "", "", "", false},
	}
	for _, tc := range cases {
		stem, profile, ext, ok := ParseOverlayName(tc.name)
		if ok != tc.ok || stem != tc.stem || profile != tc.profile || ext != tc.ext {
			t.Errorf("ParseOverlayName(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				tc.name, stem, profile, ext, ok, tc.stem, tc.profile, tc.ext, tc.ok)
		}
	}
}

func TestParseOverlayNameStrictErrors(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"archive.olay.work.tar.gz", "multi-part extension"},
		{"settings.olay.work.schema.json", "multi-part extension"},
		{".olay.work.sh", "missing stem"},
		{"script.olay..sh", "missing profile"},
	}
	for _, tc := range cases {
		_, _, _, ok, err := ParseOverlayNameStrict(tc.name)
		if err == nil || ok {
			t.Fatalf("ParseOverlayNameStrict(%q) = ok=%v err=%v, want error", tc.name, ok, err)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ParseOverlayNameStrict(%q) error = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestWalkBasicBaseAndProfile(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "dot-claude")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(pkg, "settings.olay.base.json"), `{"a":1}`)
	writeTestFile(t, filepath.Join(pkg, "settings.olay.work.json"), `{"b":2}`)

	active, inactive, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		DotPrefix:  true,
		Profiles:   []string{"work"},
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(inactive) != 0 {
		t.Errorf("expected 0 inactive, got %d", len(inactive))
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active group, got %d", len(active))
	}
	g := active[0]
	if g.Stem != "settings" {
		t.Errorf("Stem = %q", g.Stem)
	}
	if len(g.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(g.Layers))
	}
	if g.Layers[0].Profile != "base" || g.Layers[1].Profile != "work" {
		t.Errorf("wrong layer order: %+v", g.Layers)
	}
	wantTarget := filepath.Join("/tmp/out", ".claude", "settings.json")
	if g.TargetPath != wantTarget {
		t.Errorf("TargetPath = %q, want %q", g.TargetPath, wantTarget)
	}
	if g.TargetRelPath != filepath.ToSlash(filepath.Join(".claude", "settings.json")) {
		t.Errorf("TargetRelPath = %q", g.TargetRelPath)
	}
}

func TestWalkMultipleSourceRootsAreRelativeToEachRoot(t *testing.T) {
	dir := t.TempDir()
	pi := filepath.Join(dir, "pi")
	codex := filepath.Join(dir, "codex")
	writeTestFile(t, filepath.Join(pi, "dot-pi", "agent", "models.olay.base.json"), `{"pi":true}`)
	writeTestFile(t, filepath.Join(codex, "dot-codex", "config.olay.base.toml"), `model = "x"`)

	active, inactive, err := Walk(Settings{
		SourceDirs: []string{pi, codex},
		TargetDir:  "/tmp/out",
		DotPrefix:  true,
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(inactive) != 0 {
		t.Errorf("expected 0 inactive, got %d", len(inactive))
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active groups, got %d", len(active))
	}
	got := []string{active[0].TargetPath, active[1].TargetPath}
	want := []string{
		filepath.Join("/tmp/out", ".codex", "config.toml"),
		filepath.Join("/tmp/out", ".pi", "agent", "models.json"),
	}
	for _, target := range want {
		if !slices.Contains(got, target) {
			t.Errorf("missing target %q from %v", target, got)
		}
	}
	bad := filepath.Join("/tmp/out", "pi", ".pi", "agent", "models.json")
	if slices.Contains(got, bad) {
		t.Errorf("package directory leaked into target path: %v", got)
	}
}

func TestWalkDetectsTargetPathCollisionAcrossSourceRoots(t *testing.T) {
	dir := t.TempDir()
	pi := filepath.Join(dir, "pi")
	other := filepath.Join(dir, "other-pi")
	writeTestFile(t, filepath.Join(pi, "dot-pi", "agent", "models.olay.base.json"), `{"a":1}`)
	writeTestFile(t, filepath.Join(other, "dot-pi", "agent", "models.olay.base.json"), `{"a":2}`)

	_, _, err := Walk(Settings{
		SourceDirs: []string{pi, other},
		TargetDir:  "/tmp/out",
		DotPrefix:  true,
		Ignore:     NoopIgnorer(),
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "collision") || !strings.Contains(err.Error(), "models.olay.base.json") {
		t.Errorf("error should mention collision and sources: %v", err)
	}
}

func TestWalkYAMLStructuredFormats(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "config.olay.base.yaml"), `app: base`)
	writeTestFile(t, filepath.Join(dir, "theme.olay.base.yml"), `name: dark`)

	active, inactive, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(inactive) != 0 {
		t.Fatalf("expected no inactive groups, got %d", len(inactive))
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active groups, got %d", len(active))
	}
	for _, g := range active {
		if g.Format != document.FormatYAML {
			t.Fatalf("Format = %s, want yaml", g.Format)
		}
	}
	got := []string{active[0].TargetPath, active[1].TargetPath}
	want := []string{filepath.Join("/tmp/out", "config.yaml"), filepath.Join("/tmp/out", "theme.yml")}
	if !slices.Equal(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestWalkOnlyProfileNoBase(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "config.olay.work.toml"), `key = "work"`)

	active, _, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Profiles:   []string{"work"},
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 group, got %d", len(active))
	}
	if len(active[0].Layers) != 1 || active[0].Layers[0].Profile != "work" {
		t.Errorf("wrong layers: %+v", active[0].Layers)
	}
}

func TestWalkInactiveGroup(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "config.olay.work.toml"), `key = "work"`)

	active, inactive, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Profiles:   []string{"other"},
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active, got %d", len(active))
	}
	if len(inactive) != 1 {
		t.Errorf("expected 1 inactive, got %d", len(inactive))
	}
}

func TestWalkFullLayerStack(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "settings.olay.base.json"), `{"a":1}`)
	writeTestFile(t, filepath.Join(dir, "settings.olay.work.json"), `{"b":2}`)
	writeTestFile(t, filepath.Join(dir, "settings.olay.vpn.json"), `{"c":3}`)
	writeTestFile(t, filepath.Join(dir, "settings.olay.local.json"), `{"d":4}`)

	active, _, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Profiles:   []string{"work", "vpn"},
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 group, got %d", len(active))
	}
	got := []string{}
	for _, l := range active[0].Layers {
		got = append(got, l.Profile)
	}
	want := []string{"base", "work", "vpn", "local"}
	if len(got) != len(want) {
		t.Fatalf("layer count mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("layer[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWalkHiddenDirSkipped(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(hidden, "x.olay.base.json"), `{}`)

	active, _, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("hidden dir should be skipped by default, got %d groups", len(active))
	}
}

func TestWalkHiddenDirTraversed(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(hidden, "x.olay.base.json"), `{}`)

	active, _, err := Walk(Settings{
		SourceDirs:     []string{dir},
		TargetDir:      "/tmp/out",
		TraverseHidden: true,
		Ignore:         NoopIgnorer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 group with TraverseHidden, got %d", len(active))
	}
}

func TestWalkIgnorePattern(t *testing.T) {
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendor, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(vendor, "a.olay.base.json"), `{}`)
	writeTestFile(t, filepath.Join(dir, "b.olay.base.json"), `{}`)

	ign, err := NewGlobIgnorer([]string{"vendor"})
	if err != nil {
		t.Fatal(err)
	}
	active, _, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Ignore:     ign,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 group, got %d", len(active))
	}
	if active[0].Stem != "b" {
		t.Errorf("wrong group kept: %q", active[0].Stem)
	}
}

func TestWalkEmptySourceError(t *testing.T) {
	if _, _, err := Walk(Settings{TargetDir: "/tmp/out"}); err == nil {
		t.Error("expected error for empty SourceDirs")
	}
}

func TestWalkSkipsMissingSource(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	existing := filepath.Join(root, "existing")
	writeTestFile(t, filepath.Join(existing, "settings.olay.base.json"), `{"ok":true}`)

	result, err := WalkDetailed(Settings{
		SourceDirs: []string{missing, existing},
		TargetDir:  "/tmp/out",
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("WalkDetailed: %v", err)
	}
	if len(result.MissingSources) != 1 || result.MissingSources[0] != missing {
		t.Fatalf("MissingSources = %v, want [%s]", result.MissingSources, missing)
	}
	if len(result.Active) != 1 {
		t.Fatalf("expected 1 active group, got %d", len(result.Active))
	}
	if len(result.Inactive) != 0 {
		t.Fatalf("expected 0 inactive groups, got %d", len(result.Inactive))
	}
}

func TestWalkAllMissingSourcesNoop(t *testing.T) {
	root := t.TempDir()
	missingA := filepath.Join(root, "a")
	missingB := filepath.Join(root, "b")

	result, err := WalkDetailed(Settings{
		SourceDirs: []string{missingA, missingB},
		TargetDir:  "/tmp/out",
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("WalkDetailed: %v", err)
	}
	if len(result.Active) != 0 || len(result.Inactive) != 0 {
		t.Fatalf("expected no groups, got active=%d inactive=%d", len(result.Active), len(result.Inactive))
	}
	if !slices.Equal(result.MissingSources, []string{missingA, missingB}) {
		t.Fatalf("MissingSources = %v", result.MissingSources)
	}
}

func TestWalkExistingEmptySourceNoop(t *testing.T) {
	dir := t.TempDir()
	active, inactive, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(active) != 0 || len(inactive) != 0 {
		t.Fatalf("expected no groups, got active=%d inactive=%d", len(active), len(inactive))
	}
}

func TestWalkDetectsTargetPathCollision(t *testing.T) {
	// Two source groups that collapse onto the same target path:
	// dot-x/y.olay.base.json -> .x/y.json
	// .x/y.olay.base.json    -> .x/y.json  (when traverse_hidden = true)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dot-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".x"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "dot-x", "y.olay.base.json"), `{"a":1}`)
	writeTestFile(t, filepath.Join(dir, ".x", "y.olay.base.json"), `{"a":2}`)

	_, _, err := Walk(Settings{
		SourceDirs:     []string{dir},
		TargetDir:      "/tmp/out",
		DotPrefix:      true,
		TraverseHidden: true,
		Ignore:         NoopIgnorer(),
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "collision") {
		t.Errorf("error should mention collision: %v", err)
	}
}

func TestWalkCopyThroughFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "bin", "tool.olay.base.sh"), "base\n")
	writeTestFile(t, filepath.Join(dir, "bin", "tool.olay.work.sh"), "work\n")
	writeTestFile(t, filepath.Join(dir, "bin", "tool.olay.local.sh"), "local\n")

	active, inactive, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Profiles:   []string{"work"},
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(inactive) != 0 {
		t.Fatalf("expected no inactive groups, got %d", len(inactive))
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active group, got %d", len(active))
	}
	g := active[0]
	if g.Format != document.FormatCopy {
		t.Fatalf("Format = %s, want copy", g.Format)
	}
	wantTarget := filepath.Join("/tmp/out", "bin", "tool.sh")
	if g.TargetPath != wantTarget {
		t.Fatalf("TargetPath = %q, want %q", g.TargetPath, wantTarget)
	}
	if g.TargetRelPath != filepath.ToSlash(filepath.Join("bin", "tool.sh")) {
		t.Fatalf("TargetRelPath = %q", g.TargetRelPath)
	}
	got := []string{}
	for _, l := range g.Layers {
		got = append(got, l.Profile)
	}
	want := []string{"base", "work", "local"}
	if !slices.Equal(got, want) {
		t.Fatalf("layers = %v, want %v", got, want)
	}
}

func TestWalkExtensionlessCopyThroughFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "README.olay.local"), "local readme\n")

	active, _, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Ignore:     NoopIgnorer(),
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active group, got %d", len(active))
	}
	if active[0].Format != document.FormatCopy {
		t.Fatalf("Format = %s, want copy", active[0].Format)
	}
	wantTarget := filepath.Join("/tmp/out", "README")
	if active[0].TargetPath != wantTarget {
		t.Fatalf("TargetPath = %q, want %q", active[0].TargetPath, wantTarget)
	}
}

func TestWalkRejectsMultipartExtension(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "archive.olay.work.tar.gz"), "data\n")

	_, _, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Profiles:   []string{"work"},
		Ignore:     NoopIgnorer(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "multi-part extension") {
		t.Fatalf("error = %v, want multi-part extension", err)
	}
}

func TestWalkCopyThroughCollidesWithStructuredTarget(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "settings.olay.base.json"), `{"a":1}`)
	writeTestFile(t, filepath.Join(dir, "settings.json.olay.base"), "raw\n")

	_, _, err := Walk(Settings{
		SourceDirs: []string{dir},
		TargetDir:  "/tmp/out",
		Ignore:     NoopIgnorer(),
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "collision") {
		t.Fatalf("error should mention collision: %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
