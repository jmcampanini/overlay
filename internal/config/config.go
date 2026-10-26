// Package config owns overlay's configuration schema, loading, and derivation
// of the effective values commands consume.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmcampanini/go-config-loader/configloader"
	"github.com/jmcampanini/go-config-loader/pflagloader"
	"github.com/spf13/pflag"
)

const (
	// AppName is the XDG application directory and the environment prefix.
	AppName = "overlay"
	// FileName is the configuration file's base name.
	FileName = "config.toml"

	// BaseLayer is the automatic layer applied before every configured layer.
	BaseLayer = "base"
	// LocalLayer is the automatic layer applied after every configured layer.
	LocalLayer = "local"

	maxLayerNameLength = 64
)

// layerNameRE is the shared identifier grammar for user-selectable layers.
var layerNameRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Config is the config.toml schema as loaded, before validation.
type Config struct {
	Layers  []string `toml:"layers" config:"layers" pflag_singular:"layer" help:"layers to apply; --layers accepts comma-separated lists"`
	Sources []Source `toml:"source"`
}

// Source is one [[source]] entry.
type Source struct {
	Name string `toml:"name,omitempty"`
	URL  string `toml:"url"`
	Path string `toml:"path"`
}

// defaults holds every default. Layers is empty rather than nil so the TOML
// encoder writes "layers = []" in reports and bootstrapped files.
func defaults() Config {
	return Config{Layers: []string{}}
}

// Effective is the validated configuration commands consume.
type Effective struct {
	// Layers are the effective layers in application order, excluding the
	// automatic base and local layers.
	Layers  []string
	Sources []ResolvedSource
}

// ResolvedSource is a source with its name derived and its path absolute.
type ResolvedSource struct {
	Name string
	Path string
	URL  string
}

// RegisterFlags adds --layers and --layer to flags.
func RegisterFlags(flags *pflag.FlagSet) error {
	return pflagloader.Register[Config](flags)
}

// DefaultPath returns the discovered configuration file path under
// XDG_CONFIG_HOME, or ~/.config when that variable is empty.
func DefaultPath() (string, error) {
	helper, err := configloader.NewFileHelper(AppName, FileName)
	if err != nil {
		return "", err
	}

	paths := helper.XDGConfigFile()
	if len(paths) == 0 {
		return "", errors.New("locate config file: home directory is unknown")
	}
	return paths[0], nil
}

// LoadOptions selects the sources Load applies on top of the defaults.
type LoadOptions struct {
	// Env is the process environment; OVERLAY_LAYERS is read from it.
	Env map[string]string
	// Flags holds the flags RegisterFlags added, or nil when the command has none.
	Flags *pflag.FlagSet
	// Path is an explicit configuration file that must exist. When empty, the
	// default path is used and may be absent.
	Path string
}

// Load applies the configuration file, environment, and flags to the defaults
// and returns the raw configuration with its provenance report. Call Resolve
// to validate it.
func Load(opts LoadOptions) (Config, configloader.LoadReport, error) {
	fileLoader, err := newFileLoader(opts.Path)
	if err != nil {
		return Config{}, configloader.LoadReport{}, err
	}
	envLoader, err := configloader.NewEnvironmentLoader[Config](AppName, opts.Env)
	if err != nil {
		return Config{}, configloader.LoadReport{}, err
	}
	loaders := []configloader.ConfigLoader[Config]{fileLoader, envLoader}
	if opts.Flags != nil {
		flagLoader, err := pflagloader.NewLoader[Config](opts.Flags)
		if err != nil {
			return Config{}, configloader.LoadReport{}, err
		}
		loaders = append(loaders, flagLoader)
	}

	cfg, report, err := configloader.Load(defaults(), loaders...)
	if err != nil {
		return Config{}, configloader.LoadReport{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, report, nil
}

func newFileLoader(explicitPath string) (configloader.ConfigLoader[Config], error) {
	if explicitPath != "" {
		return configloader.NewRequiredFileLoader[Config](explicitPath)
	}

	discovered, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return configloader.NewMergeAllFilesLoader[Config](configloader.File(discovered))
}

// Resolve validates cfg and derives the effective configuration: layers are
// reduced to their last occurrence and validated, source names are derived
// from URLs unless set, and source paths are expanded to absolute paths.
func Resolve(cfg Config) (Effective, error) {
	layers := keepLastOccurrences(cfg.Layers)
	for _, layer := range layers {
		if err := ValidateLayerName(layer); err != nil {
			return Effective{}, err
		}
	}

	sources := make([]ResolvedSource, 0, len(cfg.Sources))
	byName := make(map[string]int, len(cfg.Sources))
	byPath := make(map[string]int, len(cfg.Sources))
	for i, source := range cfg.Sources {
		resolved, err := resolveSource(source)
		if err != nil {
			return Effective{}, fmt.Errorf("source %d: %w", i+1, err)
		}
		if prev, ok := byName[resolved.Name]; ok {
			return Effective{}, fmt.Errorf("sources %d and %d both resolve to name %q; set name on one of them", prev+1, i+1, resolved.Name)
		}
		if prev, ok := byPath[resolved.Path]; ok {
			return Effective{}, fmt.Errorf("sources %d and %d both use path %q", prev+1, i+1, resolved.Path)
		}
		byName[resolved.Name] = i
		byPath[resolved.Path] = i
		sources = append(sources, resolved)
	}

	return Effective{Layers: layers, Sources: sources}, nil
}

// LayerOrder returns the layers in application order: base, the effective
// layers, then local.
func LayerOrder(layers []string) []string {
	order := make([]string, 0, len(layers)+2)
	order = append(order, BaseLayer)
	order = append(order, layers...)
	return append(order, LocalLayer)
}

// ValidateLayerName reports why name cannot be a configured layer.
func ValidateLayerName(name string) error {
	if name == "" {
		return errors.New("layer name is blank")
	}
	if len(name) > maxLayerNameLength {
		return fmt.Errorf("layer name %q is longer than %d characters", name, maxLayerNameLength)
	}
	if !layerNameRE.MatchString(name) {
		return fmt.Errorf("layer name %q must be lowercase letters or digits separated by single hyphens", name)
	}
	if name == BaseLayer || name == LocalLayer {
		return fmt.Errorf("layer %q is automatic and cannot be listed", name)
	}
	return nil
}

// ValidateAnyLayerName reports why name cannot name a layer at all. Unlike
// ValidateLayerName it accepts the automatic base and local layers, so it
// suits places that refer to any layer in the order rather than select one.
func ValidateAnyLayerName(name string) error {
	if name == BaseLayer || name == LocalLayer {
		return nil
	}
	return ValidateLayerName(name)
}

// SourceName derives a source's default name: the last segment of its URL
// with any .git suffix removed.
func SourceName(url string) (string, error) {
	trimmed := strings.TrimRight(url, "/")
	segment := trimmed[strings.LastIndexAny(trimmed, "/:")+1:]
	name := strings.TrimSuffix(segment, ".git")
	if err := validateSourceName(name); err != nil {
		return "", fmt.Errorf("derive name from url %q: %w", url, err)
	}
	return name, nil
}

func keepLastOccurrences(items []string) []string {
	lastIndex := make(map[string]int, len(items))
	for i, item := range items {
		lastIndex[item] = i
	}

	kept := make([]string, 0, len(lastIndex))
	for i, item := range items {
		if lastIndex[item] == i {
			kept = append(kept, item)
		}
	}
	return kept
}

func resolveSource(source Source) (ResolvedSource, error) {
	if source.URL == "" {
		return ResolvedSource{}, errors.New("url is required")
	}

	name := source.Name
	if name == "" {
		derived, err := SourceName(source.URL)
		if err != nil {
			return ResolvedSource{}, err
		}
		name = derived
	} else if err := validateSourceName(name); err != nil {
		return ResolvedSource{}, fmt.Errorf("name %q: %w", name, err)
	}

	path, err := expandPath(source.Path)
	if err != nil {
		return ResolvedSource{}, err
	}

	return ResolvedSource{Name: name, Path: path, URL: source.URL}, nil
}

func validateSourceName(name string) error {
	if name == "" {
		return errors.New("source name is blank")
	}
	if strings.ContainsAny(name, "/:") || strings.ContainsFunc(name, isSpace) {
		return errors.New("source name must not contain whitespace, '/', or ':'")
	}
	return nil
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func expandPath(path string) (string, error) {
	switch {
	case path == "":
		return "", errors.New("path is required")
	case path == "~" || strings.HasPrefix(path, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand path %q: %w", path, err)
		}
		return filepath.Clean(filepath.Join(home, strings.TrimPrefix(path, "~"))), nil
	case strings.HasPrefix(path, "~"):
		return "", fmt.Errorf("path %q: only ~ and ~/ expand; ~user is not supported", path)
	case !filepath.IsAbs(path):
		return "", fmt.Errorf("path %q must be absolute or start with ~/", path)
	default:
		return filepath.Clean(path), nil
	}
}
