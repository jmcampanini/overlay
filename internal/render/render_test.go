package render

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jmcampanini/overlay/internal/config"
)

// fixture is one source checkout under construction.
type fixture struct {
	t    *testing.T
	name string
	path string
}

func newSource(t *testing.T, name, manifest string) fixture {
	t.Helper()
	f := fixture{t: t, name: name, path: t.TempDir()}
	f.file(config.ManifestFileName, manifest, 0o644)
	return f
}

func (f fixture) file(rel, content string, mode fs.FileMode) {
	f.t.Helper()
	writeFile(f.t, filepath.Join(f.path, rel), content, mode)
}

func (f fixture) resolved() config.ResolvedSource {
	return config.ResolvedSource{Name: f.name, Path: f.path, URL: "git@x:" + f.name + ".git"}
}

func writeFile(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func TestBuildSelectsListedPackagesAndMapsDotSegments(t *testing.T) {
	base := newSource(t, "dotfiles-base", "[layers.base]\npackages = [\"bash\", \"fish\"]\n")
	base.file("bash/dot-bashrc", "export A=1\n", 0o644)
	base.file("fish/dot-config/fish/conf.d/dot-/01-env.fish", "set -x A 1\n", 0o644)
	base.file("fish/dot-config/fish/dot-local-file", "x", 0o644)
	base.file("zsh/dot-zshrc", "unlisted\n", 0o644)
	target := t.TempDir()

	plan, err := Build(Request{Layers: []string{"base", "work", "local"}, Sources: []config.ResolvedSource{base.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var targets []string
	for _, entry := range plan.Entries {
		targets = append(targets, strings.TrimPrefix(entry.Target, target+"/"))
	}
	want := []string{
		".bashrc",
		".config/fish/.local-file",
		".config/fish/conf.d/dot-/01-env.fish",
	}
	if diff := cmp.Diff(want, targets); diff != "" {
		t.Errorf("Build() targets mismatch (-want +got):\n%s", diff)
	}
	first := plan.Entries[0]
	wantFirst := Entry{
		Origin:     Origin{Layer: "base", Path: "bash/dot-bashrc", Source: "dotfiles-base"},
		Kind:       KindFile,
		Mode:       0o644,
		SourceFile: filepath.Join(base.path, "bash", "dot-bashrc"),
		Status:     StatusNew,
		Target:     filepath.Join(target, ".bashrc"),
	}
	if diff := cmp.Diff(wantFirst, first); diff != "" {
		t.Errorf("Build() first entry mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"base", "work", "local"}, plan.Layers); diff != "" {
		t.Errorf("plan.Layers mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildLaterLayerReplacesEarlierAndRecordsOrigins(t *testing.T) {
	base := newSource(t, "dotfiles-base", "[layers.base]\npackages = [\"ssh\"]\n[layers.personal]\npackages = [\"ssh-personal\"]\n")
	base.file("ssh/dot-ssh/config", "Host base\n", 0o644)
	base.file("ssh-personal/dot-ssh/config", "Host base-personal\n", 0o644)
	local := newSource(t, "dotfiles-local", "[layers.local]\npackages = [\"ssh\"]\n")
	local.file("ssh/dot-ssh/config", "Host local\n", 0o600)
	target := t.TempDir()

	plan, err := Build(Request{
		Layers:  []string{"base", "personal", "local"},
		Sources: []config.ResolvedSource{base.resolved(), local.resolved()},
		Target:  target,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(plan.Entries) != 1 {
		t.Fatalf("Build() returned %d entries, want 1: %+v", len(plan.Entries), plan.Entries)
	}
	got := plan.Entries[0]
	if got.Source != "dotfiles-local" || got.Layer != "local" || got.Mode != 0o600 {
		t.Errorf("winning entry = %+v, want dotfiles-local in layer local with mode 600", got)
	}
	wantReplaces := []Origin{
		{Layer: "base", Path: "ssh/dot-ssh/config", Source: "dotfiles-base"},
		{Layer: "personal", Path: "ssh-personal/dot-ssh/config", Source: "dotfiles-base"},
	}
	if diff := cmp.Diff(wantReplaces, got.Replaces); diff != "" {
		t.Errorf("Replaces mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildFailsOnSameLayerCollision(t *testing.T) {
	tests := []struct {
		name    string
		sources func(t *testing.T) []config.ResolvedSource
	}{
		{name: "two sources", sources: func(t *testing.T) []config.ResolvedSource {
			a := newSource(t, "a", "[layers.base]\npackages = [\"ssh\"]\n")
			a.file("ssh/dot-ssh/config", "a", 0o644)
			b := newSource(t, "b", "[layers.base]\npackages = [\"ssh\"]\n")
			b.file("ssh/dot-ssh/config", "b", 0o644)
			return []config.ResolvedSource{a.resolved(), b.resolved()}
		}},
		{name: "two packages in one source", sources: func(t *testing.T) []config.ResolvedSource {
			a := newSource(t, "a", "[layers.base]\npackages = [\"ssh\", \"ssh-too\"]\n")
			a.file("ssh/dot-ssh/config", "a", 0o644)
			a.file("ssh-too/dot-ssh/config", "b", 0o644)
			return []config.ResolvedSource{a.resolved()}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := t.TempDir()

			_, err := Build(Request{Layers: []string{"base"}, Sources: tt.sources(t), Target: target})

			var collision *CollisionError
			if !errors.As(err, &collision) {
				t.Fatalf("Build() error = %v, want *CollisionError", err)
			}
			if collision.Target != filepath.Join(target, ".ssh", "config") || collision.First.Layer != "base" {
				t.Errorf("collision = %+v, want base layer at .ssh/config", collision)
			}
		})
	}
}

func TestBuildStatuses(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/new", "new\n", 0o644)
	src.file("pkg/same", "same\n", 0o644)
	src.file("pkg/content", "fresh\n", 0o644)
	src.file("pkg/mode", "mode\n", 0o600)
	src.file("pkg/link", "linked\n", 0o644)
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "same"), "same\n", 0o644)
	writeFile(t, filepath.Join(target, "content"), "stale\n", 0o644)
	writeFile(t, filepath.Join(target, "mode"), "mode\n", 0o644)
	if err := os.Symlink(filepath.Join(src.path, "pkg", "link"), filepath.Join(target, "link")); err != nil {
		t.Fatal(err)
	}

	plan, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := map[string]Status{}
	for _, entry := range plan.Entries {
		got[filepath.Base(entry.Target)] = entry.Status
	}
	want := map[string]Status{
		"new":     StatusNew,
		"same":    StatusUnchanged,
		"content": StatusChanged,
		"mode":    StatusChanged,
		"link":    StatusChanged,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("statuses mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildFollowsSymlinksToRegularFilesOnly(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("shared/real", "real\n", 0o644)
	if err := os.Mkdir(filepath.Join(src.path, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src.path, "shared", "real"), filepath.Join(src.path, "pkg", "dot-alias")); err != nil {
		t.Fatal(err)
	}

	plan, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: t.TempDir()})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(plan.Entries) != 1 || filepath.Base(plan.Entries[0].Target) != ".alias" {
		t.Fatalf("Build() entries = %+v, want one .alias entry", plan.Entries)
	}

	if err := os.Symlink(filepath.Join(src.path, "shared"), filepath.Join(src.path, "pkg", "dir-link")); err != nil {
		t.Fatal(err)
	}
	_, err = Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("Build() with a directory link error = %v, want not a regular file", err)
	}
}

func TestBuildFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (config.ResolvedSource, string)
		want  string
	}{
		{name: "missing target directory", setup: func(t *testing.T) (config.ResolvedSource, string) {
			src := newSource(t, "src", "[layers.base]\npackages = []\n")
			return src.resolved(), filepath.Join(t.TempDir(), "missing")
		}, want: "target directory"},
		{name: "missing source directory", setup: func(t *testing.T) (config.ResolvedSource, string) {
			return config.ResolvedSource{Name: "gone", Path: filepath.Join(t.TempDir(), "gone")}, t.TempDir()
		}, want: "source gone"},
		{name: "directory at target path", setup: func(t *testing.T) (config.ResolvedSource, string) {
			src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
			src.file("pkg/dot-config", "x", 0o644)
			target := t.TempDir()
			if err := os.Mkdir(filepath.Join(target, ".config"), 0o755); err != nil {
				t.Fatal(err)
			}
			return src.resolved(), target
		}, want: "is a directory"},
		{name: "dangling symlink in package", setup: func(t *testing.T) (config.ResolvedSource, string) {
			src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
			if err := os.MkdirAll(filepath.Join(src.path, "pkg"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/nonexistent/target", filepath.Join(src.path, "pkg", "broken")); err != nil {
				t.Fatal(err)
			}
			return src.resolved(), t.TempDir()
		}, want: "source src"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, target := tt.setup(t)

			_, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{source}, Target: target})

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Build() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBuildWithoutSourcesIsEmpty(t *testing.T) {
	plan, err := Build(Request{Layers: []string{"base", "local"}, Target: t.TempDir()})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(plan.Entries) != 0 {
		t.Errorf("Build() entries = %+v, want none", plan.Entries)
	}
}

func TestApplyWritesChangedFilesAndSkipsUnchanged(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/dot-config/app/settings", "fresh\n", 0o640)
	src.file("pkg/dot-secret", "secret\n", 0o600)
	src.file("pkg/same", "same\n", 0o644)
	target := t.TempDir()
	writeFile(t, filepath.Join(target, ".secret"), "old\n", 0o644)
	writeFile(t, filepath.Join(target, "same"), "same\n", 0o644)
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(target, "same"), past, past); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if err := Apply(plan); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	for _, tt := range []struct {
		rel     string
		content string
		mode    fs.FileMode
	}{
		{rel: ".config/app/settings", content: "fresh\n", mode: 0o640},
		{rel: ".secret", content: "secret\n", mode: 0o600},
		{rel: "same", content: "same\n", mode: 0o644},
	} {
		path := filepath.Join(target, tt.rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", tt.rel, err)
			continue
		}
		if string(raw) != tt.content {
			t.Errorf("%s content = %q, want %q", tt.rel, raw, tt.content)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != tt.mode {
			t.Errorf("%s mode = %o, want %o", tt.rel, info.Mode().Perm(), tt.mode)
		}
	}
	info, err := os.Stat(filepath.Join(target, "same"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Errorf("unchanged file was rewritten: mtime %v, want %v", info.ModTime(), past)
	}
	leftovers, _ := filepath.Glob(filepath.Join(target, ".*.overlay-*"))
	if len(leftovers) > 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

func TestApplyReplacesSymlinkWithoutWritingThroughIt(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/dot-bashrc", "rendered\n", 0o644)
	checkout := filepath.Join(t.TempDir(), "old-checkout", "bashrc")
	writeFile(t, checkout, "stow-managed\n", 0o644)
	target := t.TempDir()
	if err := os.Symlink(checkout, filepath.Join(target, ".bashrc")); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if err := Apply(plan); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(filepath.Join(target, ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf(".bashrc mode = %v, want a regular file", info.Mode())
	}
	if raw, _ := os.ReadFile(filepath.Join(target, ".bashrc")); string(raw) != "rendered\n" {
		t.Errorf(".bashrc content = %q, want rendered", raw)
	}
	if raw, _ := os.ReadFile(checkout); string(raw) != "stow-managed\n" {
		t.Errorf("old checkout content = %q, want untouched", raw)
	}
}

func TestApplyStopsAtFirstFailure(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/a-first", "a\n", 0o644)
	src.file("pkg/b-blocked/file", "b\n", 0o644)
	src.file("pkg/c-last", "c\n", 0o644)
	target := t.TempDir()
	blocked := filepath.Join(target, "b-blocked")
	if err := os.Mkdir(blocked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	plan, err := Build(Request{Layers: []string{"base"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	err = Apply(plan)

	if err == nil || !strings.Contains(err.Error(), "b-blocked") {
		t.Fatalf("Apply() error = %v, want failure naming b-blocked", err)
	}
	if _, err := os.Stat(filepath.Join(target, "a-first")); err != nil {
		t.Errorf("file before the failure not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "c-last")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("file after the failure exists (stat error %v), want absent", err)
	}
}

// shellenvTemplate is the loader template from issue 3: one source line per
// env.<layer> file that exists, in layer order then source order. The env
// files live outside any package so only the loader renders.
const shellenvTemplate = `test -n "$DOTFILES_LAYERS" || export DOTFILES_LAYERS='{{ join "," .Layers }}'
{{ range $layer := .Layers }}{{ range $.Sources }}
{{- $f := printf "%s/envs/env.%s" .Path $layer }}{{ if fileExists $f }}
test -f {{ shellQuote $f }} && source {{ shellQuote $f }} || test ! -f {{ shellQuote $f }} || return 1
{{ end }}{{ end }}{{ end }}
`

func templateRequest(t *testing.T, sources ...fixture) Request {
	t.Helper()
	resolved := make([]config.ResolvedSource, len(sources))
	for i, source := range sources {
		resolved[i] = source.resolved()
	}
	return Request{
		Home:     "/home/me",
		Hostname: func() (string, error) { return "wintermute", nil },
		Layers:   []string{"base", "personal", "local"},
		OS:       "linux",
		Sources:  resolved,
		Target:   t.TempDir(),
	}
}

func TestBuildRendersTheShellenvLoader(t *testing.T) {
	base := newSource(t, "dotfiles-base", "[layers.base]\npackages = [\"shellenv\"]\n")
	base.file("shellenv/dot-shellenv.tmpl", shellenvTemplate, 0o600)
	base.file("envs/env.base", "export BASE=1\n", 0o644)
	base.file("envs/env.macos", "unused layer\n", 0o644)
	personal := newSource(t, "dotfiles-personal", "[layers.personal]\npackages = []\n")
	personal.file("envs/env.personal", "export PERSONAL=1\n", 0o644)
	personal.file("envs/env.local", "export LOCAL=1\n", 0o644)
	req := templateRequest(t, base, personal)

	plan, err := Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if len(plan.Entries) != 1 {
		t.Fatalf("Build() returned %d entries, want 1: %+v", len(plan.Entries), plan.Entries)
	}
	entry := plan.Entries[0]
	if entry.Kind != KindTemplate || entry.Path != "shellenv/dot-shellenv.tmpl" || entry.Mode != 0o600 || entry.Status != StatusNew {
		t.Errorf("entry = %+v, want a new template from shellenv/dot-shellenv.tmpl with mode 600", entry)
	}
	want := "test -n \"$DOTFILES_LAYERS\" || export DOTFILES_LAYERS='base,personal,local'\n\n" +
		"test -f '" + base.path + "/envs/env.base' && source '" + base.path + "/envs/env.base' || test ! -f '" + base.path + "/envs/env.base' || return 1\n\n" +
		"test -f '" + personal.path + "/envs/env.personal' && source '" + personal.path + "/envs/env.personal' || test ! -f '" + personal.path + "/envs/env.personal' || return 1\n\n" +
		"test -f '" + personal.path + "/envs/env.local' && source '" + personal.path + "/envs/env.local' || test ! -f '" + personal.path + "/envs/env.local' || return 1\n\n"
	if diff := cmp.Diff(want, string(entry.Content)); diff != "" {
		t.Errorf("rendered loader mismatch (-want +got):\n%s", diff)
	}
	written, err := os.ReadFile(filepath.Join(req.Target, ".shellenv"))
	if err != nil {
		t.Fatalf("read rendered .shellenv: %v", err)
	}
	if string(written) != want {
		t.Errorf(".shellenv content = %q, want the rendered loader", written)
	}
	info, err := os.Stat(filepath.Join(req.Target, ".shellenv"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf(".shellenv mode = %o, want 600", info.Mode().Perm())
	}
}

func TestBuildTemplateContextAndTargetMapping(t *testing.T) {
	base := newSource(t, "dotfiles-base", "[layers.base]\npackages = [\"facts\"]\n")
	base.file("facts/dot-config/dot-facts.tmpl", "{{ .Home }} {{ .Hostname }} {{ .OS }} {{ len .Sources }} {{ (index .Sources 0).Name }}\n", 0o644)
	base.file("facts/plain.tmpl.txt", "not a template {{ .Nope }}\n", 0o644)
	req := templateRequest(t, base)

	plan, err := Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := map[string]Entry{}
	for _, entry := range plan.Entries {
		got[strings.TrimPrefix(entry.Target, req.Target+"/")] = entry
	}
	facts, ok := got[".config/.facts"]
	if !ok {
		t.Fatalf("Build() targets = %v, want .config/.facts", got)
	}
	if string(facts.Content) != "/home/me wintermute linux 1 dotfiles-base\n" {
		t.Errorf(".facts content = %q, want every context field", facts.Content)
	}
	plain, ok := got["plain.tmpl.txt"]
	if !ok || plain.Kind != KindFile || plain.Content != nil {
		t.Errorf("plain.tmpl.txt entry = %+v, want a file entry with no content", plain)
	}
}

func TestBuildTemplateStatusUsesRenderedBytes(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	src.file("pkg/same.tmpl", "host {{ .Hostname }}\n", 0o644)
	src.file("pkg/changed.tmpl", "host {{ .Hostname }}\n", 0o644)
	req := templateRequest(t, src)
	writeFile(t, filepath.Join(req.Target, "same"), "host wintermute\n", 0o644)
	writeFile(t, filepath.Join(req.Target, "changed"), "host {{ .Hostname }}\n", 0o644)

	plan, err := Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := map[string]Status{}
	for _, entry := range plan.Entries {
		got[filepath.Base(entry.Target)] = entry.Status
	}
	want := map[string]Status{"same": StatusUnchanged, "changed": StatusChanged}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("statuses mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildTemplateReplacesAndCollidesLikeFiles(t *testing.T) {
	base := newSource(t, "base", "[layers.base]\npackages = [\"pkg\"]\n")
	base.file("pkg/dot-rc", "plain\n", 0o644)
	personal := newSource(t, "personal", "[layers.personal]\npackages = [\"pkg\"]\n")
	personal.file("pkg/dot-rc.tmpl", "templated {{ .OS }}\n", 0o644)

	plan, err := Build(templateRequest(t, base, personal))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(plan.Entries) != 1 || plan.Entries[0].Kind != KindTemplate || string(plan.Entries[0].Content) != "templated linux\n" {
		t.Fatalf("Build() entries = %+v, want one template entry replacing the base file", plan.Entries)
	}
	if diff := cmp.Diff([]Origin{{Layer: "base", Path: "pkg/dot-rc", Source: "base"}}, plan.Entries[0].Replaces); diff != "" {
		t.Errorf("Replaces mismatch (-want +got):\n%s", diff)
	}

	base.file("pkg/dot-rc.tmpl", "also base\n", 0o644)
	_, err = Build(templateRequest(t, base))

	var collision *CollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("Build() with pkg/dot-rc and pkg/dot-rc.tmpl error = %v, want *CollisionError", err)
	}
}

func TestBuildTemplateFailuresWriteNothing(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "parse error", text: "{{ if .Home }}open\n", want: "unexpected EOF"},
		{name: "unknown field", text: "{{ .Target }}\n", want: "can't evaluate field Target"},
		{name: "unknown function", text: "{{ now }}\n", want: "not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
			src.file("pkg/a-good", "fine\n", 0o644)
			src.file("pkg/b-bad.tmpl", tt.text, 0o644)
			req := templateRequest(t, src)

			_, err := Build(req)

			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "src:pkg/b-bad.tmpl") {
				t.Errorf("Build() error = %v, want containing %q and the template name", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(req.Target, "a-good")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("a-good exists after a failed build (stat error %v), want absent", err)
			}
		})
	}
}

func TestBuildRejectsATemplateWithoutAName(t *testing.T) {
	for _, name := range []string{"pkg/.tmpl", "pkg/dot-.tmpl", "pkg/sub/.tmpl"} {
		t.Run(name, func(t *testing.T) {
			src := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
			src.file(name, "x", 0o644)

			_, err := Build(templateRequest(t, src))

			if err == nil || !strings.Contains(err.Error(), "needs a name") {
				t.Errorf("Build() with %s error = %v, want needs a name", name, err)
			}
		})
	}
}

func TestBuildLooksUpTheHostnameOnlyForTemplates(t *testing.T) {
	lookupErr := errors.New("no hostname")
	failing := func() (string, error) { return "", lookupErr }
	plain := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	plain.file("pkg/dot-rc", "plain\n", 0o644)
	templated := newSource(t, "src", "[layers.base]\npackages = [\"pkg\"]\n")
	templated.file("pkg/dot-rc.tmpl", "{{ .OS }}\n", 0o644)

	plainReq := templateRequest(t, plain)
	plainReq.Hostname = failing
	if _, err := Build(plainReq); err != nil {
		t.Errorf("Build() with plain files and a failing hostname lookup error = %v, want nil", err)
	}

	templatedReq := templateRequest(t, templated)
	templatedReq.Hostname = failing
	_, err := Build(templatedReq)
	if !errors.Is(err, lookupErr) {
		t.Errorf("Build() with a template and a failing hostname lookup error = %v, want %v", err, lookupErr)
	}

	templatedReq.Hostname = nil
	_, err = Build(templatedReq)
	if err == nil || !strings.Contains(err.Error(), "hostname") {
		t.Errorf("Build() with a template and no hostname lookup error = %v, want one naming the hostname", err)
	}
}

func TestBuildMergesFragmentsAcrossSourcesInLayerOrder(t *testing.T) {
	base := newSource(t, "dotfiles-base", "[layers.base]\npackages = [\"codex\"]\n")
	base.file("codex/dot-codex/config.olay.base.toml", "model = \"gpt\"\ntags = [\"base\"]\n\n[features]\napps = false\n", 0o644)
	personal := newSource(t, "dotfiles-personal", "[layers.personal]\npackages = [\"codex\"]\n")
	personal.file("codex/dot-codex/config.olay.personal.toml", "model = \"gpt-pro\"\ntags = [\"personal\"]\n\n[features]\ngoals = true\n", 0o644)
	personal.file("codex/dot-codex/config.olay.local.toml", "model = \"local\"\ntags = [\"local\"]\n\n[projects.\"${OVERLAY_TEST_HOME}/code\"]\ntrust_level = \"trusted\"\n", 0o600)
	personal.file("codex/dot-codex/config.olay.work.toml", "model = \"never\"\n", 0o644)
	t.Setenv("OVERLAY_TEST_HOME", "/home/me")
	target := t.TempDir()
	req := Request{
		Layers:  []string{"base", "personal", "local"},
		Sources: []config.ResolvedSource{base.resolved(), personal.resolved()},
		Target:  target,
	}

	plan, err := Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(plan.Entries) != 1 {
		t.Fatalf("Build() returned %d entries, want 1: %+v", len(plan.Entries), plan.Entries)
	}
	want := Entry{
		Origin:  Origin{Layer: "local", Path: "codex/dot-codex/config.olay.local.toml", Source: "dotfiles-personal"},
		Content: []byte("model = \"local\"\ntags = [\"base\", \"personal\", \"local\"]\n\n[features]\napps = false\ngoals = true\n\n[projects]\n[projects.\"/home/me/code\"]\ntrust_level = \"trusted\"\n"),
		Kind:    KindMerge,
		Merges: []Fragment{
			{Origin: Origin{Layer: "base", Path: "codex/dot-codex/config.olay.base.toml", Source: "dotfiles-base"}, SourceFile: filepath.Join(base.path, "codex/dot-codex/config.olay.base.toml")},
			{Origin: Origin{Layer: "personal", Path: "codex/dot-codex/config.olay.personal.toml", Source: "dotfiles-personal"}, SourceFile: filepath.Join(personal.path, "codex/dot-codex/config.olay.personal.toml")},
			{Origin: Origin{Layer: "local", Path: "codex/dot-codex/config.olay.local.toml", Source: "dotfiles-personal"}, SourceFile: filepath.Join(personal.path, "codex/dot-codex/config.olay.local.toml")},
		},
		Mode:       0o600,
		SourceFile: filepath.Join(personal.path, "codex/dot-codex/config.olay.local.toml"),
		Status:     StatusNew,
		Target:     filepath.Join(target, ".codex", "config.toml"),
	}
	if diff := cmp.Diff(want, plan.Entries[0]); diff != "" {
		t.Errorf("Build() merge entry mismatch (-want +got):\n%s", diff)
	}

	if err := Apply(plan); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	written, err := os.ReadFile(want.Target)
	if err != nil || string(written) != string(want.Content) {
		t.Errorf("Apply() wrote %q (%v), want the merged document", written, err)
	}
	if info, err := os.Stat(want.Target); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("Apply() mode = %v (%v), want 600 from the last fragment", info, err)
	}
	after, err := Build(req)
	if err != nil {
		t.Fatalf("second Build() error = %v", err)
	}
	if after.Entries[0].Status != StatusUnchanged {
		t.Errorf("status after Apply = %q, want %q", after.Entries[0].Status, StatusUnchanged)
	}
}

func TestBuildWholeFileAboveFragmentsReplacesThem(t *testing.T) {
	base := newSource(t, "dotfiles-base", "[layers.base]\npackages = [\"claude\"]\n")
	base.file("claude/dot-claude/settings.olay.base.json", `{"model": "opus"}`, 0o644)
	personal := newSource(t, "dotfiles-personal", "[layers.personal]\npackages = [\"claude\"]\n")
	personal.file("claude/dot-claude/settings.json", `{"model": "mine"}`, 0o644)
	target := t.TempDir()

	plan, err := Build(Request{
		Layers:  []string{"base", "personal", "local"},
		Sources: []config.ResolvedSource{base.resolved(), personal.resolved()},
		Target:  target,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(plan.Entries) != 1 {
		t.Fatalf("Build() returned %d entries, want 1: %+v", len(plan.Entries), plan.Entries)
	}
	got := plan.Entries[0]
	if got.Kind != KindFile || got.Source != "dotfiles-personal" || got.Merges != nil {
		t.Errorf("entry = %+v, want the personal file with no merges", got)
	}
	wantReplaces := []Origin{{Layer: "base", Path: "claude/dot-claude/settings.olay.base.json", Source: "dotfiles-base"}}
	if diff := cmp.Diff(wantReplaces, got.Replaces); diff != "" {
		t.Errorf("Replaces mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildFragmentFailures(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "fragment above a whole file", files: map[string]string{
			"claude/dot-claude/settings.json":               `{"model": "base"}`,
			"claude/dot-claude/settings.olay.personal.json": `{"model": "mine"}`,
		}, want: []string{"src:claude/dot-claude/settings.olay.personal.json is a fragment"}},
		{name: "fragment above a template", files: map[string]string{
			"claude/dot-claude/settings.json.tmpl":       `{"model": "base"}`,
			"claude/dot-claude/settings.olay.local.json": `{"model": "mine"}`,
		}, want: []string{"src:claude/dot-claude/settings.json.tmpl (base) renders that file whole"}},
		{name: "no name before the marker", files: map[string]string{"claude/.olay.base.json": `{}`}, want: []string{"a fragment needs a name before .olay."}},
		{name: "no extension", files: map[string]string{"claude/settings.olay.base": `{}`}, want: []string{"a fragment needs LAYER.EXT after .olay."}},
		{name: "invalid layer", files: map[string]string{"claude/settings.olay.Base.json": `{}`}, want: []string{"fragment layer: layer name \"Base\""}},
		{name: "unsupported extension", files: map[string]string{"claude/settings.olay.base.yaml": `a: 1`}, want: []string{"fragment extension \"yaml\" must be json or toml"}},
		{name: "template suffix on a fragment", files: map[string]string{"claude/settings.olay.base.json.tmpl": `{}`}, want: []string{"fragment extension \"json.tmpl\" must be json or toml"}},
		{name: "marker twice", files: map[string]string{"claude/settings.olay.base.olay.local.json": `{}`}, want: []string{"holds .olay. once"}},
		{name: "fragment that does not parse", files: map[string]string{"claude/dot-claude/settings.olay.base.json": `{"model": }`}, want: []string{"merge ", "fragment src:claude/dot-claude/settings.olay.base.json"}},
		{name: "unset variable", files: map[string]string{"claude/dot-claude/settings.olay.base.json": `{"a": "${OVERLAY_TEST_UNSET_VARIABLE}"}`}, want: []string{"variable OVERLAY_TEST_UNSET_VARIABLE is not set"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newSource(t, "src", "[layers.base]\npackages = [\"claude\"]\n")
			for name, content := range tt.files {
				src.file(name, content, 0o644)
			}
			target := t.TempDir()

			_, err := Build(Request{Layers: []string{"base", "personal", "local"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})

			if err == nil {
				t.Fatalf("Build() error = nil, want containing %q", tt.want)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Build() error = %v, want containing %q", err, want)
				}
			}
		})
	}
}

func TestBuildFragmentsInOneLayerCollide(t *testing.T) {
	a := newSource(t, "a", "[layers.base]\npackages = [\"claude\"]\n")
	a.file("claude/settings.olay.base.json", `{}`, 0o644)
	b := newSource(t, "b", "[layers.personal]\npackages = [\"claude\"]\n")
	b.file("claude/settings.olay.base.json", `{}`, 0o644)
	target := t.TempDir()

	_, err := Build(Request{Layers: []string{"base", "personal", "local"}, Sources: []config.ResolvedSource{a.resolved(), b.resolved()}, Target: target})

	var collision *CollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("Build() error = %v, want *CollisionError", err)
	}
	if collision.First.Source != "a" || collision.Second.Source != "b" || collision.First.Layer != "base" || collision.Second.Layer != "base" {
		t.Errorf("collision = %+v, want both base fragments named", collision)
	}
}

func TestBuildPackageListedUnderTwoLayersContributesEachFragmentOnce(t *testing.T) {
	src := newSource(t, "src", "[layers.base]\npackages = [\"claude\"]\n[layers.personal]\npackages = [\"claude\"]\n")
	src.file("claude/settings.olay.base.json", `{"a": 1}`, 0o644)
	src.file("claude/settings.olay.personal.json", `{"b": 2}`, 0o644)
	target := t.TempDir()

	plan, err := Build(Request{Layers: []string{"base", "personal", "local"}, Sources: []config.ResolvedSource{src.resolved()}, Target: target})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(plan.Entries) != 1 || len(plan.Entries[0].Merges) != 2 {
		t.Fatalf("Build() entries = %+v, want one merge of two fragments", plan.Entries)
	}
	if got := string(plan.Entries[0].Content); got != "{\n  \"a\": 1,\n  \"b\": 2\n}\n" {
		t.Errorf("Content = %q, want both fragments merged once", got)
	}
}
