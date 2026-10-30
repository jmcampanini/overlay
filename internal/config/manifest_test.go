package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadManifestReadsLayersAndPackages(t *testing.T) {
	source := t.TempDir()
	mkdirs(t, source, "bash", "ssh", "macos")
	writeManifest(t, source, `[layers.base]
packages = ["bash", "ssh"]

[layers.local]
packages = []

[layers.mac-os]
packages = ["macos"]
`)

	manifest, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	want := Manifest{Layers: map[string]ManifestLayer{
		"base":   {Packages: []string{"bash", "ssh"}},
		"local":  {Packages: []string{}},
		"mac-os": {Packages: []string{"macos"}},
	}}
	if diff := cmp.Diff(want, manifest); diff != "" {
		t.Errorf("LoadManifest() mismatch (-want +got):\n%s", diff)
	}
	if got := manifest.Packages("unlisted"); got != nil {
		t.Errorf("Packages(unlisted) = %v, want nil", got)
	}
}

func TestLoadManifestFailures(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     string
	}{
		{name: "unknown key", manifest: "[layers.base]\npackages = [\"bash\"]\nextra = 1\n", want: "unknown keys: layers.base.extra"},
		{name: "unknown top-level table", manifest: "[profiles.base]\npackages = [\"bash\"]\n", want: "unknown keys"},
		{name: "invalid layer name", manifest: "[layers.Work]\npackages = [\"bash\"]\n", want: "lowercase"},
		{name: "missing package", manifest: "[layers.base]\npackages = [\"zsh\"]\n", want: `package "zsh"`},
		{name: "package is a file", manifest: "[layers.base]\npackages = [\"README\"]\n", want: "not a directory"},
		{name: "nested package", manifest: "[layers.base]\npackages = [\"bash/sub\"]\n", want: "top-level directory"},
		{name: "blank package", manifest: "[layers.base]\npackages = [\"\"]\n", want: "blank"},
		{name: "package under two layers", manifest: "[layers.base]\npackages = [\"bash\"]\n[layers.work]\npackages = [\"bash\"]\n", want: `package "bash" is listed under layers "base" and "work"`},
		{name: "package twice in one layer", manifest: "[layers.base]\npackages = [\"bash\", \"bash\"]\n", want: `layer "base" lists package "bash" twice`},
		{name: "malformed toml", manifest: "[layers.base\n", want: "read manifest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := t.TempDir()
			mkdirs(t, source, "bash")
			if err := os.WriteFile(filepath.Join(source, "README"), []byte("hi"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeManifest(t, source, tt.manifest)

			_, err := LoadManifest(source)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("LoadManifest() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadManifestRequiresTheFile(t *testing.T) {
	source := t.TempDir()

	_, err := LoadManifest(source)

	if err == nil || !strings.Contains(err.Error(), ManifestFileName) {
		t.Errorf("LoadManifest() error = %v, want it to name %s", err, ManifestFileName)
	}
}

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
}

func writeManifest(t *testing.T, source, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(source, ManifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
