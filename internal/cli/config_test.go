package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"github.com/jmcampanini/overlay/internal/config"
)

const fixtureConfig = `layers = ["personal", "server", "personal"]

[[source]]
url = "git@github.com:jmcampanini/dotfiles-base.git"
path = "~/dotfiles/base"

[[source]]
name = "extra"
url = "git@github.com:jmcampanini/dotfiles-personal.git"
path = "/srv/personal"
`

func TestConfigReportRoundTripsAndListsEffectiveValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	configPath := filepath.Join(configDir, "overlay", "config.toml")
	writeConfig(t, configPath, fixtureConfig)

	out, errOut, err := executeForTest(t, "config")
	if err != nil {
		t.Fatalf("config error = %v, stderr = %s", err, errOut)
	}

	body, effective, found := strings.Cut(out, "\n# Effective\n")
	if !found {
		t.Fatalf("config output has no # Effective block:\n%s", out)
	}
	var reloaded config.Config
	if _, err := toml.Decode(body, &reloaded); err != nil {
		t.Fatalf("decode report body: %v\n%s", err, body)
	}
	var original config.Config
	if _, err := toml.Decode(fixtureConfig, &original); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if diff := cmp.Diff(original, reloaded); diff != "" {
		t.Errorf("report body does not reload as the file (-want +got):\n%s", diff)
	}
	wantEffective := strings.Join([]string{
		`# loaded_files = ["` + configPath + `"]`,
		`# effective_layers = ["server", "personal"]`,
		`# layer_order = ["base", "server", "personal", "local"]`,
		`# source dotfiles-base = "` + filepath.Join(home, "dotfiles", "base") + `"`,
		`# source extra = "/srv/personal"`,
	}, "\n") + "\n"
	if effective != wantEffective {
		t.Errorf("effective block mismatch:\n got %q\nwant %q", effective, wantEffective)
	}
}

func TestConfigProvenanceNamesEachSource(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	configPath := filepath.Join(configDir, "overlay", "config.toml")
	writeConfig(t, configPath, fixtureConfig)
	t.Setenv("OVERLAY_LAYERS", "work")

	out, errOut, err := executeForTest(t, "config", "--provenance")
	if err != nil {
		t.Fatalf("config --provenance error = %v, stderr = %s", err, errOut)
	}

	_, provenance, found := strings.Cut(out, "\n# Provenance\n")
	if !found {
		t.Fatalf("config output has no # Provenance block:\n%s", out)
	}
	for _, want := range []struct{ path, source string }{
		{path: "layers", source: "<env>"},
		{path: "sources", source: configPath},
	} {
		if !rowHasSource(provenance, want.path, want.source) {
			t.Errorf("provenance lacks row %s from %s:\n%s", want.path, want.source, provenance)
		}
	}
}

func TestConfigReportsDefaultsWithoutAFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out, errOut, err := executeForTest(t, "config")
	if err != nil {
		t.Fatalf("config error = %v, stderr = %s", err, errOut)
	}

	for _, want := range []string{"layers = []\n", "# loaded_files = []\n", `# layer_order = ["base", "local"]` + "\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("config output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigFailures(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing explicit file", args: []string{"--config", filepath.Join(t.TempDir(), "missing.toml")}, want: "missing.toml"},
		{name: "empty explicit file", args: []string{"--config", ""}, want: "--config must name a file"},
		{name: "automatic layer selected", args: []string{"--layers", "base"}, want: "automatic"},
		{name: "invalid layer from flag", args: []string{"--layer", "Work"}, want: "lowercase"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())

			out, _, err := executeForTest(t, append([]string{"config"}, tt.args...)...)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("config %v error = %v, want containing %q", tt.args, err, tt.want)
			}
			if out != "" {
				t.Errorf("config %v wrote to stdout on failure:\n%s", tt.args, out)
			}
		})
	}
}

func TestConfigAppendsEnvironmentLayersAfterFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		raw  []string
		want string
	}{
		{name: "environment replaces file", raw: []string{"environment"}, want: `["environment", "laptop", "work"]`},
		{name: "flags replace environment", args: []string{"--layers", "desktop,work"}, raw: []string{"desktop", "work"}, want: `["desktop", "laptop", "work"]`},
		{name: "empty flags keep additions", args: []string{"--layers", ""}, raw: []string{}, want: `["laptop", "work"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")
			writeConfig(t, configPath, `layers = ["file"]
env_layers = ["OVERLAY_TEST_HOST_LAYERS", "OVERLAY_TEST_EMPTY_LAYERS", "OVERLAY_TEST_SESSION_LAYERS"]
`)
			t.Setenv("OVERLAY_LAYERS", "environment")
			t.Setenv("OVERLAY_ENV_LAYERS", "OVERLAY_TEST_WRONG_LAYERS")
			t.Setenv("OVERLAY_TEST_WRONG_LAYERS", "wrong")
			t.Setenv("OVERLAY_TEST_HOST_LAYERS", " work, laptop ,, ")
			t.Setenv("OVERLAY_TEST_EMPTY_LAYERS", "")
			t.Setenv("OVERLAY_TEST_SESSION_LAYERS", "laptop, work,")

			args := append([]string{"config", "--config", configPath, "--provenance"}, tt.args...)
			out, errOut, err := executeForTest(t, args...)
			if err != nil {
				t.Fatalf("config error = %v, stderr = %s", err, errOut)
			}

			var reloaded config.Config
			if _, err := toml.Decode(out, &reloaded); err != nil {
				t.Fatalf("decode config report: %v", err)
			}
			if diff := cmp.Diff(tt.raw, reloaded.Layers); diff != "" {
				t.Errorf("raw layers mismatch (-want +got):\n%s", diff)
			}
			wantNames := []string{"OVERLAY_TEST_HOST_LAYERS", "OVERLAY_TEST_EMPTY_LAYERS", "OVERLAY_TEST_SESSION_LAYERS"}
			if diff := cmp.Diff(wantNames, reloaded.EnvLayers); diff != "" {
				t.Errorf("env_layers mismatch (-want +got):\n%s", diff)
			}
			if want := "# effective_layers = " + tt.want + "\n"; !strings.Contains(out, want) {
				t.Errorf("config output missing %q:\n%s", want, out)
			}
			if !rowHasSource(out, "envlayers", configPath) {
				t.Errorf("env_layers provenance does not name %q:\n%s", configPath, out)
			}
		})
	}
}

func TestConfigRejectsInvalidEnvironmentLayers(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
		want  string
	}{
		{name: "blank variable", field: "", want: "variable name"},
		{name: "whitespace variable", field: " ", want: "variable name"},
		{name: "padded variable", field: " OVERLAY_TEST_LAYERS", want: "variable name"},
		{name: "uppercase layer", field: "OVERLAY_TEST_LAYERS", value: "Work", want: "lowercase"},
		{name: "path layer", field: "OVERLAY_TEST_LAYERS", value: "../work", want: "lowercase"},
		{name: "long layer", field: "OVERLAY_TEST_LAYERS", value: strings.Repeat("x", 65), want: "longer than 64"},
		{name: "automatic base", field: "OVERLAY_TEST_LAYERS", value: "base", want: "automatic"},
		{name: "automatic local", field: "OVERLAY_TEST_LAYERS", value: "local", want: "automatic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")
			writeConfig(t, configPath, fmt.Sprintf("env_layers = [%q]\n", tt.field))
			t.Setenv("OVERLAY_TEST_LAYERS", tt.value)
			t.Setenv("OVERLAY_LAYERS", "")

			out, _, err := executeForTest(t, "config", "--config", configPath)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("config error = %v, want containing %q", err, tt.want)
			}
			if out != "" {
				t.Errorf("config wrote to stdout on failure:\n%s", out)
			}
		})
	}
}

func TestConfigIgnoresRelativeXDGConfigHome(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	t.Setenv("OVERLAY_LAYERS", "")
	writeConfig(t, filepath.Join("relative-config", "overlay", "config.toml"), `unexpected = true`)
	configPath := filepath.Join(home, ".config", "overlay", "config.toml")
	writeConfig(t, configPath, `layers = []`)

	out, errOut, err := executeForTest(t, "config")
	if err != nil {
		t.Fatalf("config error = %v, stderr = %s", err, errOut)
	}

	if want := fmt.Sprintf("# loaded_files = [%q]\n", configPath); !strings.Contains(out, want) {
		t.Errorf("config output missing %q:\n%s", want, out)
	}
}

func rowHasSource(table, path, source string) bool {
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == path && strings.HasSuffix(line, source) {
			return true
		}
	}
	return false
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
