package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// renderFixture is a two-source checkout pair with a config file naming them.
type renderFixture struct {
	configPath string
	home       string
}

// newRenderFixture builds dotfiles-base (bash and ssh under base) and
// dotfiles-personal (ssh under personal) in temp directories and points HOME
// and the config file at them. The base ssh config is a symlink target in
// home so the symlink-replacement contract is exercised end to end.
func newRenderFixture(t *testing.T) renderFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "")
	base := t.TempDir()
	personal := t.TempDir()
	writeFixtureFile(t, filepath.Join(base, "overlay.toml"), "[layers.base]\npackages = [\"bash\", \"ssh\"]\n", 0o644)
	writeFixtureFile(t, filepath.Join(base, "bash", "dot-bashrc"), "export A=1\n", 0o644)
	writeFixtureFile(t, filepath.Join(base, "ssh", "dot-ssh", "config"), "Host base\n", 0o644)
	writeFixtureFile(t, filepath.Join(base, "unlisted", "dot-ignored"), "no\n", 0o644)
	writeFixtureFile(t, filepath.Join(personal, "overlay.toml"), "[layers.personal]\npackages = [\"ssh\"]\n", 0o644)
	writeFixtureFile(t, filepath.Join(personal, "ssh", "dot-ssh", "config"), "Host personal\n", 0o600)
	writeFixtureFile(t, filepath.Join(home, ".bashrc"), "export A=1\n", 0o644)

	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeConfig(t, configPath, "layers = [\"personal\"]\n\n[[source]]\nurl = \"git@x:dotfiles-base.git\"\npath = \""+base+"\"\n\n[[source]]\nurl = \"git@x:dotfiles-personal.git\"\npath = \""+personal+"\"\n")
	return renderFixture{configPath: configPath, home: home}
}

const fixtureTable = `TARGET         SOURCE                                KIND  STATUS     REPLACES
~/.bashrc      dotfiles-base:bash/dot-bashrc         file  unchanged
~/.ssh/config  dotfiles-personal:ssh/dot-ssh/config  file  new        dotfiles-base:ssh/dot-ssh/config (base)

2 files: 1 new, 0 changed, 1 unchanged
`

func TestRenderDryRunPrintsThePlanWithoutWriting(t *testing.T) {
	f := newRenderFixture(t)

	out, errOut, err := executeForTest(t, "render", "--config", f.configPath, "--dry-run")
	if err != nil {
		t.Fatalf("render --dry-run error = %v, stderr = %s", err, errOut)
	}

	if out != fixtureTable {
		t.Errorf("dry-run output mismatch:\n got:\n%s\nwant:\n%s", out, fixtureTable)
	}
	requireNoFile(t, filepath.Join(f.home, ".ssh", "config"))
}

func TestRenderWritesFilesAndPrintsTheSamePlan(t *testing.T) {
	f := newRenderFixture(t)

	out, errOut, err := executeForTest(t, "render", "--config", f.configPath)
	if err != nil {
		t.Fatalf("render error = %v, stderr = %s", err, errOut)
	}

	if out != fixtureTable {
		t.Errorf("render output mismatch:\n got:\n%s\nwant:\n%s", out, fixtureTable)
	}
	sshConfig := filepath.Join(f.home, ".ssh", "config")
	raw, err := os.ReadFile(sshConfig)
	if err != nil {
		t.Fatalf("read rendered ssh config: %v", err)
	}
	if string(raw) != "Host personal\n" {
		t.Errorf("ssh config = %q, want the personal layer's file", raw)
	}
	info, err := os.Stat(sshConfig)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("ssh config mode = %o, want 600 copied from the source", info.Mode().Perm())
	}
	requireNoFile(t, filepath.Join(f.home, ".ignored"))

	again, _, err := executeForTest(t, "render", "--config", f.configPath)
	if err != nil {
		t.Fatalf("second render error = %v", err)
	}
	if !strings.Contains(again, "2 files: 0 new, 0 changed, 2 unchanged") {
		t.Errorf("second render is not a no-op:\n%s", again)
	}
}

func TestRenderJSONDocument(t *testing.T) {
	f := newRenderFixture(t)

	out, errOut, err := executeForTest(t, "render", "--config", f.configPath, "--dry-run", "--json", "--color", "always")
	if err != nil {
		t.Fatalf("render --json error = %v, stderr = %s", err, errOut)
	}

	if strings.Contains(out, "\x1b") {
		t.Errorf("JSON output contains escapes:\n%q", out)
	}
	var doc planDocument
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	want := planDocument{
		Target: f.home,
		Layers: []string{"base", "personal", "local"},
		Entries: []planEntry{
			{Target: filepath.Join(f.home, ".bashrc"), Source: "dotfiles-base", Path: "bash/dot-bashrc", Layer: "base", Kind: "file", Status: "unchanged", Replaces: []planOrigin{}, Fragments: []planOrigin{}},
			{Target: filepath.Join(f.home, ".ssh", "config"), Source: "dotfiles-personal", Path: "ssh/dot-ssh/config", Layer: "personal", Kind: "file", Status: "new",
				Replaces: []planOrigin{{Source: "dotfiles-base", Path: "ssh/dot-ssh/config", Layer: "base"}}, Fragments: []planOrigin{}},
		},
	}
	if diff := cmp.Diff(want, doc); diff != "" {
		t.Errorf("JSON document mismatch (-want +got):\n%s", diff)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Errorf("JSON output is not one document line:\n%s", out)
	}
}

func TestRenderColorFollowsThePolicyOnRedirectedOutput(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		env         map[string]string
		wantEscapes bool
	}{
		{name: "default redirected", wantEscapes: false},
		{name: "always", args: []string{"--color", "always"}, wantEscapes: true},
		{name: "always beats NO_COLOR", args: []string{"--color", "always"}, env: map[string]string{"NO_COLOR": "1"}, wantEscapes: true},
		{name: "never", args: []string{"--color", "never"}, wantEscapes: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRenderFixture(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			out, errOut, err := executeForTest(t, append([]string{"render", "--config", f.configPath, "--dry-run"}, tt.args...)...)
			if err != nil {
				t.Fatalf("render error = %v, stderr = %s", err, errOut)
			}

			if got := strings.Contains(out, "\x1b["); got != tt.wantEscapes {
				t.Errorf("output has escapes = %t, want %t:\n%q", got, tt.wantEscapes, out)
			}
			if tt.wantEscapes && !strings.Contains(out, "\x1b[32mnew") {
				t.Errorf("new status is not green:\n%q", out)
			}
		})
	}
}

func TestRenderTemplatesShowTheirKindAndRenderedContent(t *testing.T) {
	f := newRenderFixture(t)
	base := baseSourcePath(t, f)
	writeFixtureFile(t, filepath.Join(base, "overlay.toml"), "[layers.base]\npackages = [\"bash\", \"ssh\", \"facts\"]\n", 0o644)
	writeFixtureFile(t, filepath.Join(base, "facts", "dot-facts.tmpl"), "{{ .OS }} {{ join \"+\" .Layers }} {{ (index .Sources 1).Name }}\n", 0o600)
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(base, "facts", "host.tmpl"), "{{ .Hostname }} {{ .Home }}\n", 0o644)

	out, errOut, err := executeForTest(t, "render", "--config", f.configPath)
	if err != nil {
		t.Fatalf("render error = %v, stderr = %s", err, errOut)
	}

	for _, line := range []string{
		"~/.facts       dotfiles-base:facts/dot-facts.tmpl    template  new",
		"~/host         dotfiles-base:facts/host.tmpl         template  new",
		"4 files: 3 new, 0 changed, 1 unchanged",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("output lacks %q:\n%s", line, out)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(f.home, ".facts")); err != nil || string(raw) != runtime.GOOS+" base+personal+local dotfiles-personal\n" {
		t.Errorf(".facts = %q (%v), want the rendered facts", raw, err)
	}
	if raw, err := os.ReadFile(filepath.Join(f.home, "host")); err != nil || string(raw) != hostname+" "+f.home+"\n" {
		t.Errorf("host = %q (%v), want hostname and home", raw, err)
	}
	if info, err := os.Stat(filepath.Join(f.home, ".facts")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf(".facts mode = %v (%v), want 600 from the template", info, err)
	}
}

func TestRenderMergesValuesFromTheDirectoryThenTheFlag(t *testing.T) {
	f := newRenderFixture(t)
	writeFixtureFile(t, filepath.Join(valuesDir(), "10-theme.toml"), "[theme]\nname = \"gruvbox\"\nbg = \"#282828\"\n", 0o644)
	writeFixtureFile(t, filepath.Join(valuesDir(), "50-local.toml"), "[theme]\nbg = \"#000000\"\n", 0o644)
	writeFixtureFile(t, filepath.Join(valuesDir(), "notes.txt"), "[theme]\nname = \"ignored\"\n", 0o644)
	extra := filepath.Join(t.TempDir(), "extra.toml")
	writeFixtureFile(t, extra, "[theme]\nname = \"nord\"\n", 0o644)
	writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-theme.tmpl"), "{{ .Values.theme.name }} {{ .Values.theme.bg }}\n", 0o644)

	_, errOut, err := executeForTest(t, "render", "--config", f.configPath, "--values", extra)
	if err != nil {
		t.Fatalf("render error = %v, stderr = %s", err, errOut)
	}

	if raw, err := os.ReadFile(filepath.Join(f.home, ".theme")); err != nil || string(raw) != "nord #000000\n" {
		t.Errorf(".theme = %q (%v), want the flag's name over the directory's and the later file's bg", raw, err)
	}
}

func TestRenderTargetFlag(t *testing.T) {
	f := newRenderFixture(t)
	target := filepath.Join(f.home, "staging")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := executeForTest(t, "render", "--config", f.configPath, "--target", "~/staging", "--layers", "")
	if err != nil {
		t.Fatalf("render --target error = %v, stderr = %s", err, errOut)
	}

	if !strings.Contains(out, "~/staging/.ssh/config  dotfiles-base:ssh/dot-ssh/config") {
		t.Errorf("output does not show the base ssh config under the target:\n%s", out)
	}
	if raw, err := os.ReadFile(filepath.Join(target, ".ssh", "config")); err != nil || string(raw) != "Host base\n" {
		t.Errorf("staging ssh config = %q (%v), want the base layer's file", raw, err)
	}
	requireNoFile(t, filepath.Join(f.home, ".ssh", "config"))
}

func TestResolveTarget(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		target  string
		want    string
		wantErr string
	}{
		{target: "", want: "/home/me"},
		{target: "~", want: "/home/me"},
		{target: "~/tmp/home", want: "/home/me/tmp/home"},
		{target: "/srv/home", want: "/srv/home"},
		{target: "staging", want: filepath.Join(cwd, "staging")},
		{target: "~other/x", wantErr: "~user is not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got, err := resolveTarget(tt.target, "/home/me")

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("resolveTarget(%q) error = %v, want containing %q", tt.target, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("resolveTarget(%q) = %q, %v, want %q", tt.target, got, err, tt.want)
			}
		})
	}
}

func TestRenderFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, f renderFixture) []string
		want  string
	}{
		{name: "same-layer collision", setup: func(t *testing.T, f renderFixture) []string {
			t.Helper()
			other := t.TempDir()
			writeFixtureFile(t, filepath.Join(other, "overlay.toml"), "[layers.base]\npackages = [\"bash\"]\n", 0o644)
			writeFixtureFile(t, filepath.Join(other, "bash", "dot-bashrc"), "export B=2\n", 0o644)
			writeConfig(t, f.configPath, "[[source]]\nurl = \"git@x:dotfiles-base.git\"\npath = \""+baseSourcePath(t, f)+"\"\n\n[[source]]\nurl = \"git@x:other.git\"\npath = \""+other+"\"\n")
			return nil
		}, want: "both render to"},
		{name: "missing target", setup: func(_ *testing.T, f renderFixture) []string {
			return []string{"--target", filepath.Join(f.home, "missing")}
		}, want: "target directory"},
		{name: "missing manifest", setup: func(t *testing.T, f renderFixture) []string {
			t.Helper()
			writeConfig(t, f.configPath, "[[source]]\nurl = \"git@x:bare.git\"\npath = \""+t.TempDir()+"\"\n")
			return nil
		}, want: "overlay.toml"},
		{name: "template that fails to execute", setup: func(t *testing.T, f renderFixture) []string {
			t.Helper()
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-profile.tmpl"), "{{ .Missing }}\n", 0o644)
			return []string{"--dry-run"}
		}, want: "dotfiles-base:bash/dot-profile.tmpl"},
		{name: "fragment that fails to parse", setup: func(t *testing.T, f renderFixture) []string {
			t.Helper()
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-claude", "settings.olay.base.json"), "{\n", 0o644)
			return []string{"--dry-run"}
		}, want: "fragment dotfiles-base:bash/dot-claude/settings.olay.base.json: parse JSON"},
		{name: "templated fragment that fails to execute", setup: func(t *testing.T, f renderFixture) []string {
			t.Helper()
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-claude", "settings.olay.base.json.tmpl"), "{\"token\": \"{{ readFile \"/nonexistent/overlay-test-token\" }}\"}\n", 0o644)
			return []string{"--dry-run"}
		}, want: "fragment dotfiles-base:bash/dot-claude/settings.olay.base.json.tmpl: render template: dotfiles-base:bash/dot-claude/settings.olay.base.json.tmpl:1:"},
		{name: "fragment above a whole file", setup: func(t *testing.T, f renderFixture) []string {
			t.Helper()
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "overlay.toml"), "[layers.base]\npackages = [\"bash\", \"ssh\"]\n[layers.personal]\n", 0o644)
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-claude", "settings.json"), "{}\n", 0o644)
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-claude", "settings.olay.personal.json"), "{}\n", 0o644)
			return []string{"--dry-run"}
		}, want: "renders that file whole in an earlier layer"},
		{name: "invalid color mode", setup: func(_ *testing.T, _ renderFixture) []string {
			return []string{"--color", "sometimes"}
		}, want: "invalid color mode"},
		{name: "automatic layer selected", setup: func(_ *testing.T, _ renderFixture) []string {
			return []string{"--layer", "local"}
		}, want: "automatic"},
		{name: "values file that fails to parse", setup: func(t *testing.T, _ renderFixture) []string {
			t.Helper()
			writeFixtureFile(t, filepath.Join(valuesDir(), "bad.toml"), "theme =\n", 0o644)
			return []string{"--dry-run"}
		}, want: "bad.toml: parse TOML"},
		{name: "template in the values directory", setup: func(t *testing.T, _ renderFixture) []string {
			t.Helper()
			writeFixtureFile(t, filepath.Join(valuesDir(), "theme.toml.tmpl"), "", 0o644)
			return []string{"--dry-run"}
		}, want: "cannot be a template"},
		{name: "missing --values path", setup: func(t *testing.T, _ renderFixture) []string {
			return []string{"--dry-run", "--values", filepath.Join(t.TempDir(), "missing.toml")}
		}, want: "--values: stat"},
		{name: "template reads a value no file set", setup: func(t *testing.T, f renderFixture) []string {
			t.Helper()
			writeFixtureFile(t, filepath.Join(baseSourcePath(t, f), "bash", "dot-theme.tmpl"), "{{ .Values.theme }}\n", 0o644)
			return []string{"--dry-run"}
		}, want: "map has no entry for key \"theme\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRenderFixture(t)
			args := tt.setup(t, f)

			out, _, err := executeForTest(t, append([]string{"render", "--config", f.configPath}, args...)...)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("render %v error = %v, want containing %q", args, err, tt.want)
			}
			if out != "" {
				t.Errorf("render %v wrote to stdout on failure:\n%s", args, out)
			}
			requireNoFile(t, filepath.Join(f.home, ".ssh", "config"))
		})
	}
}

// valuesDir is the values directory the fixture's XDG_CONFIG_HOME selects.
func valuesDir() string {
	return filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "overlay", "values")
}

func baseSourcePath(t *testing.T, f renderFixture) string {
	t.Helper()
	cfg := readConfig(t, f.configPath)
	return cfg.Sources[0].Path
}

func writeFixtureFile(t *testing.T, path, content string, mode fs.FileMode) {
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

func TestRenderMergesFragmentsAndShowsEveryFragment(t *testing.T) {
	f := newRenderFixture(t)
	base := baseSourcePath(t, f)
	personal := readConfig(t, f.configPath).Sources[1].Path
	writeFixtureFile(t, filepath.Join(base, "overlay.toml"), "[layers.base]\npackages = [\"bash\", \"ssh\", \"claude\"]\n", 0o644)
	writeFixtureFile(t, filepath.Join(base, "claude", "dot-claude", "settings.olay.base.json"), "{\"permissions\": {\"deny\": [\"Read(a)\"]}, \"model\": \"opus\"}\n", 0o644)
	writeFixtureFile(t, filepath.Join(base, "claude", "dot-claude", "settings.olay.work.json"), "{\"model\": \"inactive\"}\n", 0o644)
	writeFixtureFile(t, filepath.Join(personal, "overlay.toml"), "[layers.personal]\npackages = [\"ssh\", \"claude\"]\n", 0o644)
	writeFixtureFile(t, filepath.Join(personal, "claude", "dot-claude", "settings.olay.personal.json.tmpl"), "{\"permissions\": {\"deny\": [\"Read(b)\"]}, \"env\": {\"URL\": \"{{ .Values.proxy }}\", \"HOME\": \"{{ .Home }}\"}}\n", 0o600)
	valuesPath := filepath.Join(t.TempDir(), "values.toml")
	writeFixtureFile(t, valuesPath, `proxy = "http://proxy"`+"\n", 0o644)

	out, errOut, err := executeForTest(t, "render", "--config", f.configPath, "--values", valuesPath)
	if err != nil {
		t.Fatalf("render error = %v, stderr = %s", err, errOut)
	}

	wantRow := "~/.claude/settings.json  dotfiles-base:claude/dot-claude/settings.olay.base.json, dotfiles-personal:claude/dot-claude/settings.olay.personal.json.tmpl  merge  new"
	if !strings.Contains(out, wantRow) {
		t.Errorf("output lacks %q:\n%s", wantRow, out)
	}
	want := "{\n  \"env\": {\n    \"HOME\": \"" + f.home + "\",\n    \"URL\": \"http://proxy\"\n  },\n  \"model\": \"opus\",\n  \"permissions\": {\n    \"deny\": [\n      \"Read(a)\",\n      \"Read(b)\"\n    ]\n  }\n}\n"
	if raw, err := os.ReadFile(filepath.Join(f.home, ".claude", "settings.json")); err != nil || string(raw) != want {
		t.Errorf("settings.json = %q (%v), want %q", raw, err, want)
	}
	if info, err := os.Stat(filepath.Join(f.home, ".claude", "settings.json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("settings.json mode = %v (%v), want 600, the bits every fragment has", info, err)
	}

	out, errOut, err = executeForTest(t, "render", "--config", f.configPath, "--values", valuesPath, "--dry-run", "--json")
	if err != nil {
		t.Fatalf("render --json error = %v, stderr = %s", err, errOut)
	}
	var doc planDocument
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	wantEntry := planEntry{
		Target: filepath.Join(f.home, ".claude", "settings.json"), Source: "dotfiles-personal", Path: "claude/dot-claude/settings.olay.personal.json.tmpl", Layer: "personal",
		Kind: "merge", Status: "unchanged", Replaces: []planOrigin{},
		Fragments: []planOrigin{
			{Source: "dotfiles-base", Path: "claude/dot-claude/settings.olay.base.json", Layer: "base"},
			{Source: "dotfiles-personal", Path: "claude/dot-claude/settings.olay.personal.json.tmpl", Layer: "personal"},
		},
	}
	if diff := cmp.Diff(wantEntry, doc.Entries[1]); diff != "" {
		t.Errorf("JSON merge entry mismatch (-want +got):\n%s", diff)
	}
}

func TestRenderConcatenatesFragmentsAndShowsEveryFragment(t *testing.T) {
	f := newRenderFixture(t)
	base := baseSourcePath(t, f)
	personal := readConfig(t, f.configPath).Sources[1].Path
	writeFixtureFile(t, filepath.Join(base, "overlay.toml"), "[layers.base]\npackages = [\"bash\", \"ssh\", \"zsh\"]\n", 0o644)
	writeFixtureFile(t, filepath.Join(base, "zsh", "dot-zshrc.olay.base"), "export BASE=1", 0o644)
	writeFixtureFile(t, filepath.Join(personal, "overlay.toml"), "[layers.personal]\npackages = [\"ssh\", \"zsh\"]\n", 0o644)
	writeFixtureFile(t, filepath.Join(personal, "zsh", "dot-zshrc.olay.personal.tmpl"), "export HOME_COPY={{ .Home }}\n", 0o600)

	out, errOut, err := executeForTest(t, "render", "--config", f.configPath)
	if err != nil {
		t.Fatalf("render error = %v, stderr = %s", err, errOut)
	}

	wantRow := "~/.zshrc       dotfiles-base:zsh/dot-zshrc.olay.base, dotfiles-personal:zsh/dot-zshrc.olay.personal.tmpl  concat  new"
	if !strings.Contains(out, wantRow) {
		t.Errorf("output lacks %q:\n%s", wantRow, out)
	}
	want := "export BASE=1\nexport HOME_COPY=" + f.home + "\n"
	if raw, err := os.ReadFile(filepath.Join(f.home, ".zshrc")); err != nil || string(raw) != want {
		t.Errorf(".zshrc = %q (%v), want %q", raw, err, want)
	}

	out, errOut, err = executeForTest(t, "render", "--config", f.configPath, "--dry-run", "--json")
	if err != nil {
		t.Fatalf("render --json error = %v, stderr = %s", err, errOut)
	}
	var doc planDocument
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	wantEntry := planEntry{
		Target: filepath.Join(f.home, ".zshrc"), Source: "dotfiles-personal", Path: "zsh/dot-zshrc.olay.personal.tmpl", Layer: "personal",
		Kind: "concat", Status: "unchanged", Replaces: []planOrigin{},
		Fragments: []planOrigin{
			{Source: "dotfiles-base", Path: "zsh/dot-zshrc.olay.base", Layer: "base"},
			{Source: "dotfiles-personal", Path: "zsh/dot-zshrc.olay.personal.tmpl", Layer: "personal"},
		},
	}
	if diff := cmp.Diff(wantEntry, doc.Entries[2]); diff != "" {
		t.Errorf("JSON concat entry mismatch (-want +got):\n%s", diff)
	}
}

func TestRenderUsesOnlyDeclaredFragmentLayers(t *testing.T) {
	for _, declared := range []bool{false, true} {
		name := "undeclared"
		if declared {
			name = "declared"
		}
		t.Run(name, func(t *testing.T) {
			f := newRenderFixture(t)
			base := baseSourcePath(t, f)
			manifest := "[layers.base]\npackages = [\"bash\"]\n"
			if declared {
				manifest += "[layers.work]\n[layers.local]\n"
			}
			writeFixtureFile(t, filepath.Join(base, "overlay.toml"), manifest, 0o644)
			writeFixtureFile(t, filepath.Join(base, "bash", "settings.olay.base.json"), `{"base": true}`, 0o644)
			writeFixtureFile(t, filepath.Join(base, "bash", "settings.olay.work.json"), `{"work": true}`, 0o644)
			writeFixtureFile(t, filepath.Join(base, "bash", "settings.olay.local.json"), `{"local": true}`, 0o644)

			_, _, err := executeForTest(t, "render", "--config", f.configPath, "--layers", "work")
			if err != nil {
				t.Fatalf("render error = %v", err)
			}

			data, err := os.ReadFile(filepath.Join(f.home, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]bool
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			want := map[string]bool{"base": true}
			if declared {
				want["work"] = true
				want["local"] = true
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("fragment layers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRenderAndDiffApplyEnvironmentLayersAfterFlags(t *testing.T) {
	f := newRenderFixture(t)
	data, err := os.ReadFile(f.configPath)
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, f.configPath, "env_layers = [\"OVERLAY_TEST_SESSION_LAYERS\"]\n"+string(data))
	t.Setenv("OVERLAY_TEST_SESSION_LAYERS", "personal")

	patch, _, err := executeForTest(t, "diff", "--config", f.configPath, "--layers", "")
	if err != nil {
		t.Fatalf("diff error = %v", err)
	}
	if !strings.Contains(patch, "+Host personal\n") {
		t.Errorf("diff output = %q, want personal layer content", patch)
	}
	_, _, err = executeForTest(t, "render", "--config", f.configPath, "--layers", "")
	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(f.home, ".ssh", "config")); err != nil || string(got) != "Host personal\n" {
		t.Errorf("rendered content = %q (%v), want personal layer content", got, err)
	}
}
