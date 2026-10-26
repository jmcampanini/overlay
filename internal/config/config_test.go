package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
)

func TestResolveKeepsLastOccurrenceOfEachLayer(t *testing.T) {
	tests := []struct {
		name   string
		layers []string
		want   []string
	}{
		{name: "empty", layers: []string{}, want: []string{}},
		{name: "unique", layers: []string{"work", "laptop"}, want: []string{"work", "laptop"}},
		{name: "repeated", layers: []string{"work", "laptop", "work"}, want: []string{"laptop", "work"}},
		{name: "repeated twice", layers: []string{"a", "b", "a", "b"}, want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(Config{Layers: tt.layers}, nil)
			if err != nil {
				t.Fatalf("Resolve(%v) error = %v", tt.layers, err)
			}
			if diff := cmp.Diff(tt.want, got.Layers); diff != "" {
				t.Errorf("Resolve(%v).Layers mismatch (-want +got):\n%s", tt.layers, diff)
			}
		})
	}
}

func TestResolveRejectsInvalidLayerNames(t *testing.T) {
	tests := []struct {
		layer string
		want  string
	}{
		{layer: "", want: "blank"},
		{layer: " ", want: "lowercase"},
		{layer: "Work", want: "lowercase"},
		{layer: "-work", want: "lowercase"},
		{layer: "work--laptop", want: "lowercase"},
		{layer: "work_laptop", want: "lowercase"},
		{layer: strings.Repeat("a", 65), want: "longer than 64"},
		{layer: "base", want: "automatic"},
		{layer: "local", want: "automatic"},
	}
	for _, tt := range tests {
		t.Run(tt.layer, func(t *testing.T) {
			_, err := Resolve(Config{Layers: []string{"ok", tt.layer}}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Resolve with layer %q error = %v, want containing %q", tt.layer, err, tt.want)
			}
		})
	}
}

func TestResolveAppendsEnvLayers(t *testing.T) {
	tests := []struct {
		name      string
		layers    []string
		envLayers []string
		env       map[string]string
		want      []string
	}{
		{
			name:      "variables read in list order",
			layers:    []string{"work"},
			envLayers: []string{"HOST_LAYERS", "SESSION_LAYERS"},
			env:       map[string]string{"SESSION_LAYERS": "session", "HOST_LAYERS": "host"},
			want:      []string{"work", "host", "session"},
		},
		{
			name:      "values split on commas, trimmed, blanks dropped",
			envLayers: []string{"HOST_LAYERS"},
			env:       map[string]string{"HOST_LAYERS": " laptop , ,, work ,"},
			want:      []string{"laptop", "work"},
		},
		{
			name:      "unset variable adds nothing",
			layers:    []string{"work"},
			envLayers: []string{"HOST_LAYERS"},
			want:      []string{"work"},
		},
		{
			name:      "empty variable adds nothing",
			layers:    []string{"work"},
			envLayers: []string{"HOST_LAYERS"},
			env:       map[string]string{"HOST_LAYERS": ""},
			want:      []string{"work"},
		},
		{
			name:      "additions apply to cleared raw layers",
			layers:    []string{},
			envLayers: []string{"HOST_LAYERS"},
			env:       map[string]string{"HOST_LAYERS": "host"},
			want:      []string{"host"},
		},
		{
			name:      "last occurrence wins across raw and env layers",
			layers:    []string{"work", "laptop"},
			envLayers: []string{"HOST_LAYERS", "SESSION_LAYERS"},
			env:       map[string]string{"HOST_LAYERS": "work,host", "SESSION_LAYERS": "host"},
			want:      []string{"laptop", "work", "host"},
		},
		{
			name:      "variable names match exactly",
			envLayers: []string{"HOST_LAYERS"},
			env:       map[string]string{"host_layers": "lower", "OVERLAY_HOST_LAYERS": "prefixed", "HOST_LAYERS ": "padded"},
			want:      []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(Config{EnvLayers: tt.envLayers, Layers: tt.layers}, tt.env)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if diff := cmp.Diff(tt.want, got.Layers); diff != "" {
				t.Errorf("Resolve().Layers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveRejectsInvalidEnvLayers(t *testing.T) {
	tests := []struct {
		name      string
		envLayers []string
		env       map[string]string
		want      string
	}{
		{name: "blank variable name", envLayers: []string{"HOST_LAYERS", ""}, want: "env_layers item 2 is blank"},
		{name: "whitespace variable name", envLayers: []string{" "}, want: "env_layers item 1 is blank"},
		{name: "padded variable name", envLayers: []string{" HOST_LAYERS"}, env: map[string]string{" HOST_LAYERS": "work"}, want: "surrounding whitespace"},
		{name: "base through env", envLayers: []string{"HOST_LAYERS"}, env: map[string]string{"HOST_LAYERS": "base"}, want: `env_layers variable HOST_LAYERS: layer "base" is automatic`},
		{name: "local through env", envLayers: []string{"SESSION_LAYERS"}, env: map[string]string{"SESSION_LAYERS": "work,local"}, want: `env_layers variable SESSION_LAYERS: layer "local" is automatic`},
		{name: "invalid grammar through env", envLayers: []string{"HOST_LAYERS"}, env: map[string]string{"HOST_LAYERS": "Work"}, want: `env_layers variable HOST_LAYERS: layer name "Work" must be lowercase`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(Config{EnvLayers: tt.envLayers, Layers: []string{"ok"}}, tt.env)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Resolve() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestResolveSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name    string
		source  Source
		want    ResolvedSource
		wantErr string
	}{
		{
			name:   "ssh url derives name",
			source: Source{URL: "git@github.com:jmcampanini/dotfiles-base.git", Path: "/srv/base"},
			want:   ResolvedSource{Name: "dotfiles-base", Path: "/srv/base", URL: "git@github.com:jmcampanini/dotfiles-base.git"},
		},
		{
			name:   "https url without .git",
			source: Source{URL: "https://github.com/jmcampanini/dotfiles", Path: "/srv/dotfiles/"},
			want:   ResolvedSource{Name: "dotfiles", Path: "/srv/dotfiles", URL: "https://github.com/jmcampanini/dotfiles"},
		},
		{
			name:   "trailing slash is ignored",
			source: Source{URL: "https://example.com/repo.git/", Path: "/srv/repo"},
			want:   ResolvedSource{Name: "repo", Path: "/srv/repo", URL: "https://example.com/repo.git/"},
		},
		{
			name:   "explicit name wins",
			source: Source{Name: "custom", URL: "git@example.com:a/b.git", Path: "/srv/b"},
			want:   ResolvedSource{Name: "custom", Path: "/srv/b", URL: "git@example.com:a/b.git"},
		},
		{
			name:   "tilde slash expands",
			source: Source{URL: "git@example.com:a/b.git", Path: "~/dotfiles/b"},
			want:   ResolvedSource{Name: "b", Path: filepath.Join(home, "dotfiles", "b"), URL: "git@example.com:a/b.git"},
		},
		{
			name:   "bare tilde expands",
			source: Source{URL: "git@example.com:a/b.git", Path: "~"},
			want:   ResolvedSource{Name: "b", Path: home, URL: "git@example.com:a/b.git"},
		},
		{name: "tilde user is rejected", source: Source{URL: "git@example.com:a/b.git", Path: "~alice/b"}, wantErr: "~user is not supported"},
		{name: "relative path is rejected", source: Source{URL: "git@example.com:a/b.git", Path: "dotfiles/b"}, wantErr: "must be absolute"},
		{name: "missing path", source: Source{URL: "git@example.com:a/b.git"}, wantErr: "path is required"},
		{name: "missing url", source: Source{Path: "/srv/b"}, wantErr: "url is required"},
		{name: "url with no name", source: Source{URL: "https://", Path: "/srv/b"}, wantErr: "blank"},
		{name: "name with space", source: Source{Name: "my repo", URL: "git@example.com:a/b.git", Path: "/srv/b"}, wantErr: "whitespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(Config{Layers: []string{}, Sources: []Source{tt.source}}, nil)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Resolve(%+v) error = %v, want containing %q", tt.source, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%+v) error = %v", tt.source, err)
			}
			if diff := cmp.Diff([]ResolvedSource{tt.want}, got.Sources); diff != "" {
				t.Errorf("Resolve(%+v).Sources mismatch (-want +got):\n%s", tt.source, diff)
			}
		})
	}
}

func TestResolveRejectsCollidingSources(t *testing.T) {
	sameName := []Source{
		{URL: "git@a.example.com:x/dotfiles.git", Path: "/srv/a"},
		{URL: "git@b.example.com:y/dotfiles.git", Path: "/srv/b"},
	}
	_, err := Resolve(Config{Sources: sameName}, nil)
	if err == nil || !strings.Contains(err.Error(), `name "dotfiles"`) {
		t.Errorf("Resolve with duplicate names error = %v, want name collision", err)
	}

	renamed := []Source{sameName[0], {Name: "dotfiles-b", URL: sameName[1].URL, Path: sameName[1].Path}}
	if _, err := Resolve(Config{Sources: renamed}, nil); err != nil {
		t.Errorf("Resolve with explicit name error = %v, want nil", err)
	}

	samePath := []Source{
		{URL: "git@a.example.com:x/one.git", Path: "/srv/shared"},
		{URL: "git@b.example.com:y/two.git", Path: "/srv/shared/"},
	}
	_, err = Resolve(Config{Sources: samePath}, nil)
	if err == nil || !strings.Contains(err.Error(), `path "/srv/shared"`) {
		t.Errorf("Resolve with duplicate paths error = %v, want path collision", err)
	}
}

func TestLayerOrderWrapsLayersInAutomaticLayers(t *testing.T) {
	got := LayerOrder([]string{"work", "laptop"})

	want := []string{"base", "work", "laptop", "local"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LayerOrder mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadPrecedence(t *testing.T) {
	const fileLayers = `layers = ["file", "shared"]` + "\n"
	tests := []struct {
		name       string
		file       string
		env        map[string]string
		args       []string
		want       []string
		wantSource string
	}{
		{name: "file only", file: fileLayers, want: []string{"file", "shared"}},
		{name: "env replaces file", file: fileLayers, env: map[string]string{"OVERLAY_LAYERS": "env"}, want: []string{"env"}, wantSource: "<env>"},
		{name: "flags replace env", file: fileLayers, env: map[string]string{"OVERLAY_LAYERS": "env"}, args: []string{"--layers", "flag"}, want: []string{"flag"}, wantSource: "<pflag>"},
		{name: "empty flag clears", file: fileLayers, args: []string{"--layers", ""}, want: []string{}, wantSource: "<pflag>"},
		{name: "env keeps last occurrence", env: map[string]string{"OVERLAY_LAYERS": "work, laptop ,work"}, want: []string{"laptop", "work"}, wantSource: "<env>"},
		{name: "plural before singular", args: []string{"--layer", "work", "--layers", "work,laptop"}, want: []string{"laptop", "work"}, wantSource: "<pflag>"},
		{name: "singular repeats last", args: []string{"--layers", "a,b", "--layer", "a"}, want: []string{"b", "a"}, wantSource: "<pflag>"},
		{name: "file repeats reduce", file: `layers = ["a", "b", "a"]` + "\n", want: []string{"b", "a"}},
		{name: "no file uses defaults", want: []string{}, wantSource: "<default>"},
		{
			name: "env_layers append after flags clear",
			file: fileLayers + `env_layers = ["HOST_LAYERS"]` + "\n",
			env:  map[string]string{"HOST_LAYERS": "host"},
			args: []string{"--layers", ""},
			want: []string{"host"}, wantSource: "<pflag>",
		},
		{
			name: "env_layers append after env replaces file",
			file: fileLayers + `env_layers = ["HOST_LAYERS"]` + "\n",
			env:  map[string]string{"OVERLAY_LAYERS": "env,host", "HOST_LAYERS": "laptop"},
			want: []string{"env", "host", "laptop"}, wantSource: "<env>",
		},
		{
			name: "OVERLAY_ENV_LAYERS is not read",
			file: fileLayers,
			env:  map[string]string{"OVERLAY_ENV_LAYERS": "HOST_LAYERS", "HOST_LAYERS": "host"},
			want: []string{"file", "shared"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configDir)
			path := filepath.Join(configDir, "overlay", "config.toml")
			if tt.file != "" {
				writeFile(t, path, tt.file)
			}
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			if err := RegisterFlags(flags); err != nil {
				t.Fatalf("RegisterFlags() error = %v", err)
			}
			if err := flags.Parse(tt.args); err != nil {
				t.Fatalf("Parse(%v) error = %v", tt.args, err)
			}

			cfg, report, err := Load(LoadOptions{Env: tt.env, Flags: flags})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			effective, err := Resolve(cfg, tt.env)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if diff := cmp.Diff(tt.want, effective.Layers); diff != "" {
				t.Errorf("effective layers mismatch (-want +got):\n%s", diff)
			}
			wantSource := tt.wantSource
			if wantSource == "" {
				wantSource = path
			}
			if got := report.Updates["layers"]; got != wantSource {
				t.Errorf("report.Updates[layers] = %q, want %q", got, wantSource)
			}
		})
	}
}

func TestLoadRejectsBlankLayerItems(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
	}{
		{name: "blank inside csv", args: []string{"--layers", "work,,laptop"}},
		{name: "empty after value", args: []string{"--layers", "work", "--layers", ""}},
		{name: "whitespace env", env: map[string]string{"OVERLAY_LAYERS": "  "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			if err := RegisterFlags(flags); err != nil {
				t.Fatalf("RegisterFlags() error = %v", err)
			}
			if err := flags.Parse(tt.args); err != nil {
				t.Fatalf("Parse(%v) error = %v", tt.args, err)
			}

			cfg, _, err := Load(LoadOptions{Env: tt.env, Flags: flags})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			_, err = Resolve(cfg, tt.env)

			if err == nil || !strings.Contains(err.Error(), "blank") {
				t.Errorf("Resolve() error = %v, want blank layer name", err)
			}
		})
	}
}

func TestRegisterFlagsAddsNoEnvLayersFlag(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := RegisterFlags(flags); err != nil {
		t.Fatalf("RegisterFlags() error = %v", err)
	}

	if flags.Lookup("env-layers") != nil {
		t.Error("RegisterFlags() added --env-layers; env_layers is file-only")
	}
}

func TestLoadExplicitPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeFile(t, filepath.Join(dir, "overlay", "config.toml"), `layers = ["discovered"]`+"\n")
	explicit := filepath.Join(dir, "explicit.toml")
	writeFile(t, explicit, `layers = ["explicit"]`+"\n")

	cfg, report, err := Load(LoadOptions{Path: explicit})
	if err != nil {
		t.Fatalf("Load(explicit) error = %v", err)
	}
	if diff := cmp.Diff([]string{"explicit"}, cfg.Layers); diff != "" {
		t.Errorf("Load(explicit) layers mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{explicit}, report.LoadedFiles); diff != "" {
		t.Errorf("Load(explicit) loaded files mismatch (-want +got):\n%s", diff)
	}

	_, _, err = Load(LoadOptions{Path: filepath.Join(dir, "missing.toml")})
	if err == nil {
		t.Error("Load(missing explicit path) error = nil, want error")
	}
}

func TestLoadReadsSourcesFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeFile(t, filepath.Join(dir, "overlay", "config.toml"), `
layers = ["personal"]
env_layers = ["HOST_LAYERS", "SESSION_LAYERS"]

[[source]]
url = "git@github.com:jmcampanini/dotfiles-base.git"
path = "/srv/base"

[[source]]
name = "extra"
url = "git@github.com:jmcampanini/dotfiles-personal.git"
path = "/srv/personal"
`)

	cfg, _, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := Config{
		EnvLayers: []string{"HOST_LAYERS", "SESSION_LAYERS"},
		Layers:    []string{"personal"},
		Sources: []Source{
			{URL: "git@github.com:jmcampanini/dotfiles-base.git", Path: "/srv/base"},
			{Name: "extra", URL: "git@github.com:jmcampanini/dotfiles-personal.git", Path: "/srv/personal"},
		},
	}
	if diff := cmp.Diff(want, cfg); diff != "" {
		t.Errorf("Load() mismatch (-want +got):\n%s", diff)
	}
}

func TestDefaultPathFollowsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	if want := filepath.Join(dir, "overlay", "config.toml"); got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
