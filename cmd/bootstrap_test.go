package cmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/gittest"
)

func TestBootstrapClonesSourcesAndWritesConfig(t *testing.T) {
	bare := gittest.NewBareRepo(t)
	dest := filepath.Join(t.TempDir(), "checkout")
	configPath := filepath.Join(t.TempDir(), "nested", "config.toml")

	out, errOut, err := executeForTest(t, "bootstrap",
		"--config", configPath,
		"--source-url", bare, "--source-path", dest,
		"--layers", "personal,server")
	if err != nil {
		t.Fatalf("bootstrap error = %v, stderr = %s", err, errOut)
	}

	wantOut := "cloned fixture into " + dest + "\nwrote " + configPath + "\n"
	if out != wantOut {
		t.Errorf("bootstrap stdout = %q, want %q", out, wantOut)
	}
	if origin := strings.TrimSpace(gittest.Run(t, dest, "remote", "get-url", "origin")); origin != bare {
		t.Errorf("origin of clone = %q, want %q", origin, bare)
	}
	written := readConfig(t, configPath)
	want := config.Config{
		Layers:  []string{"personal", "server"},
		Sources: []config.Source{{URL: bare, Path: dest}},
	}
	if diff := cmp.Diff(want, written); diff != "" {
		t.Errorf("written config mismatch (-want +got):\n%s", diff)
	}
}

func TestBootstrapWritesEmptyLayersToDefaultPath(t *testing.T) {
	bare := gittest.NewBareRepo(t)
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	dest := filepath.Join(t.TempDir(), "checkout")

	_, errOut, err := executeForTest(t, "bootstrap", "--source-url", bare, "--source-path", dest)
	if err != nil {
		t.Fatalf("bootstrap error = %v, stderr = %s", err, errOut)
	}

	configPath := filepath.Join(configDir, "overlay", "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read default config: %v", err)
	}
	if !strings.HasPrefix(string(raw), "layers = []\n") {
		t.Errorf("default config = %q, want it to start with layers = []", raw)
	}
}

func TestBootstrapLeavesExistingCheckoutAlone(t *testing.T) {
	bare := gittest.NewBareRepo(t)
	dest := filepath.Join(t.TempDir(), "checkout")
	gittest.Run(t, filepath.Dir(dest), "clone", "-q", bare, dest)
	configPath := filepath.Join(t.TempDir(), "config.toml")

	out, errOut, err := executeForTest(t, "bootstrap", "--config", configPath, "--source-url", bare, "--source-path", dest)
	if err != nil {
		t.Fatalf("bootstrap error = %v, stderr = %s", err, errOut)
	}

	if want := "exists fixture at " + dest + "\n"; !strings.HasPrefix(out, want) {
		t.Errorf("bootstrap stdout = %q, want prefix %q", out, want)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file not written: %v", err)
	}
}

func TestBootstrapRejectsPathsThatAreNotTheSource(t *testing.T) {
	bare := gittest.NewBareRepo(t)
	otherCheckout := filepath.Join(t.TempDir(), "other")
	gittest.Run(t, filepath.Dir(otherCheckout), "clone", "-q", bare, otherCheckout)
	plainDir := t.TempDir()

	tests := []struct {
		name string
		url  string
		path string
		want string
	}{
		{name: "checkout of another url", url: bare + "-other", path: otherCheckout, want: "is a checkout of"},
		{name: "plain directory", url: bare, path: plainDir, want: "not a checkout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")

			_, _, err := executeForTest(t, "bootstrap", "--config", configPath, "--source-url", tt.url, "--source-path", tt.path)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("bootstrap error = %v, want containing %q", err, tt.want)
			}
			requireNoFile(t, configPath)
		})
	}
}

func TestBootstrapRefusesToOverwriteConfig(t *testing.T) {
	bare := gittest.NewBareRepo(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeConfig(t, configPath, "layers = [\"keep\"]\n")
	dest := filepath.Join(t.TempDir(), "checkout")

	_, _, err := executeForTest(t, "bootstrap", "--config", configPath, "--source-url", bare, "--source-path", dest)

	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("bootstrap error = %v, want already exists", err)
	}
	if raw, _ := os.ReadFile(configPath); string(raw) != "layers = [\"keep\"]\n" {
		t.Errorf("existing config changed to %q", raw)
	}
	requireNoFile(t, dest)
}

func TestBootstrapLeavesNoConfigWhenCloneFails(t *testing.T) {
	gittest.NewBareRepo(t)
	missingRepo := filepath.Join(t.TempDir(), "missing.git")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, errOut, err := executeForTest(t, "bootstrap", "--config", configPath, "--source-url", missingRepo, "--source-path", filepath.Join(t.TempDir(), "dest"))

	if err == nil || !strings.Contains(err.Error(), "clone") {
		t.Errorf("bootstrap error = %v, want clone failure", err)
	}
	if !strings.Contains(errOut, "fatal") {
		t.Errorf("git output not streamed to stderr: %q", errOut)
	}
	requireNoFile(t, configPath)
}

func TestBootstrapRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no sources", args: nil, want: "at least one"},
		{name: "unpaired flags", args: []string{"--source-url", "a", "--source-url", "b", "--source-path", "/p"}, want: "pair by position"},
		{name: "relative path", args: []string{"--source-url", "git@x:a/b.git", "--source-path", "rel"}, want: "must be absolute"},
		{name: "invalid layer", args: []string{"--source-url", "git@x:a/b.git", "--source-path", "/p", "--layers", "base"}, want: "automatic"},
		{name: "blank layer", args: []string{"--source-url", "git@x:a/b.git", "--source-path", "/p", "--layers", "a,,b"}, want: "blank"},
		{name: "name collision", args: []string{"--source-url", "git@x:a/b.git", "--source-path", "/p", "--source-url", "git@y:c/b.git", "--source-path", "/q"}, want: "set name on one"},
		{name: "operand", args: []string{"extra"}, want: "unknown command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")

			_, _, err := executeForTest(t, append([]string{"bootstrap", "--config", configPath}, tt.args...)...)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("bootstrap %v error = %v, want containing %q", tt.args, err, tt.want)
			}
			requireNoFile(t, configPath)
		})
	}
}

func readConfig(t *testing.T, path string) config.Config {
	t.Helper()
	var cfg config.Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return cfg
}

func requireNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s exists (stat error %v), want absent", path, err)
	}
}
