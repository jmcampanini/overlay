package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// ManifestFileName is the manifest each source keeps at its root.
const ManifestFileName = "overlay.toml"

// Manifest is a source's overlay.toml: the packages each layer contributes.
type Manifest struct {
	Layers map[string]ManifestLayer `toml:"layers"`
}

// ManifestLayer lists the packages one layer renders from a source.
type ManifestLayer struct {
	Packages []string `toml:"packages"`
}

// Packages returns the packages listed for layer, or nil when the manifest does
// not mention it.
func (m Manifest) Packages(layer string) []string {
	return m.Layers[layer].Packages
}

// LoadManifest reads and validates the overlay.toml at the root of sourceDir.
// Every layer name must follow the layer grammar (base and local are allowed),
// every listed package must be a directory directly under sourceDir listed
// once in the whole manifest, and unknown keys are errors.
func LoadManifest(sourceDir string) (Manifest, error) {
	path := filepath.Join(sourceDir, ManifestFileName)
	var manifest Manifest
	meta, err := toml.DecodeFile(path, &manifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %q: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, key := range undecoded {
			keys[i] = key.String()
		}
		return Manifest{}, fmt.Errorf("manifest %q: unknown keys: %s", path, strings.Join(keys, ", "))
	}

	listedUnder := make(map[string]string)
	for _, name := range slices.Sorted(maps.Keys(manifest.Layers)) {
		if err := ValidateAnyLayerName(name); err != nil {
			return Manifest{}, fmt.Errorf("manifest %q: %w", path, err)
		}
		for _, pkg := range manifest.Layers[name].Packages {
			if err := validatePackage(sourceDir, pkg); err != nil {
				return Manifest{}, fmt.Errorf("manifest %q: layer %q: %w", path, name, err)
			}
			if earlier, listed := listedUnder[pkg]; listed {
				return Manifest{}, fmt.Errorf("manifest %q: %w", path, listedTwiceError(pkg, earlier, name))
			}
			listedUnder[pkg] = name
		}
	}
	return manifest, nil
}

// LoadManifests loads the manifest of every source in order, naming the
// source in any failure.
func LoadManifests(sources []ResolvedSource) ([]Manifest, error) {
	manifests := make([]Manifest, len(sources))
	for i, source := range sources {
		manifest, err := LoadManifest(source.Path)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", source.Name, err)
		}
		manifests[i] = manifest
	}
	return manifests, nil
}

func listedTwiceError(pkg, earlier, later string) error {
	if earlier == later {
		return fmt.Errorf("layer %q lists package %q twice; list each package once", later, pkg)
	}
	return fmt.Errorf("package %q is listed under layers %q and %q; list each package under one layer", pkg, earlier, later)
}

func validatePackage(sourceDir, pkg string) error {
	if pkg == "" {
		return errors.New("package name is blank")
	}
	if pkg != filepath.Base(pkg) || pkg == "." || pkg == ".." {
		return fmt.Errorf("package %q must be a top-level directory name", pkg)
	}
	info, err := os.Stat(filepath.Join(sourceDir, pkg))
	if err != nil {
		return fmt.Errorf("package %q: %w", pkg, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("package %q is not a directory", pkg)
	}
	return nil
}
