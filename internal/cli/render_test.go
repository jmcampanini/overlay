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

func TestRenderAndDiffApplyEnvLayersAfterFlagsClearTheList(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"render", "--dry-run"}, want: fixtureTable},
		{args: []string{"diff"}, want: fixturePatch},
	}
	for _, tt := range tests {
		t.Run(tt.args[0], func(t *testing.T) {
			f := newRenderFixture(t)
			original, err := os.ReadFile(f.configPath)
			if err != nil {
				t.Fatal(err)
			}
			writeConfig(t, f.configPath, "env_layers = [\"HOST_LAYERS\"]\n"+string(original))
			t.Setenv("HOST_LAYERS", "personal")

			out, errOut, err := executeForTest(t, append(append([]string{}, tt.args...), "--config", f.configPath, "--layers", "")...)
			if err != nil {
				t.Fatalf("%v error = %v, stderr = %s", tt.args, err, errOut)
			}

			if out != tt.want {
				t.Errorf("%v output = %q, want %q", tt.args, out, tt.want)
			}
			requireNoFile(t, filepath.Join(f.home, ".ssh", "config"))
		})
	}
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
			{Target: filepath.Join(f.home, ".bashrc"), Source: "dotfiles-base", Path: "bash/dot-bashrc", Layer: "base", Kind: "file", Status: "unchanged", Replaces: []planOrigin{}},
			{Target: filepath.Join(f.home, ".ssh", "config"), Source: "dotfiles-personal", Path: "ssh/dot-ssh/config", Layer: "personal", Kind: "file", Status: "new",
				Replaces: []planOrigin{{Source: "dotfiles-base", Path: "ssh/dot-ssh/config", Layer: "base"}}},
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
		{name: "invalid color mode", setup: func(_ *testing.T, _ renderFixture) []string {
			return []string{"--color", "sometimes"}
		}, want: "invalid color mode"},
		{name: "automatic layer selected", setup: func(_ *testing.T, _ renderFixture) []string {
			return []string{"--layer", "local"}
		}, want: "automatic"},
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
