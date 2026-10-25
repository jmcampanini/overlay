package cli

import (
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
