// Package config loads and validates Overlay raw configuration.
package config

// DefaultFilename is the conventional location for the overlay config.
const DefaultFilename = ".overlay.toml"

// Config mirrors the .overlay.toml schema. Fields tagged with config are
// loadable from environment variables and pflags; TOML-only fields are not.
type Config struct {
	Sources          []string     `toml:"sources" config:"sources" pflag_singular:"source" help:"override source directories from config"`
	Target           string       `toml:"target" config:"target" help:"override target directory from config"`
	DotPrefix        bool         `toml:"dot_prefix"`
	Profiles         []string     `toml:"profiles" config:"profiles" pflag_singular:"profile" help:"profile list; --profiles takes comma-separated values"`
	EnvProfiles      []string     `toml:"env_profiles"`
	ContinueOnError  bool         `toml:"continue_on_error" config:"continue" help:"continue past invalid source files"`
	TOMLIndentTables bool         `toml:"toml_indent_tables"`
	Ignore           []string     `toml:"ignore"`
	TraverseHidden   bool         `toml:"traverse_hidden"`
	RespectGitignore bool         `toml:"respect_gitignore"`
	RenderRules      []RenderRule `toml:"render_rules"`

	// Substitute is the variable-substitution switch: a non-empty list enables
	// substitution, for every target, of ${NAME} references whose variable
	// names match an exact-name or terminal-wildcard selector.
	Substitute []string `toml:"substitute"`

	// SubstituteExclude opts targets out of substitution by doublestar glob,
	// matched against the rendered target-relative path. It is the inverse of
	// the substitute switch and mirrors the ignore field.
	SubstituteExclude []string `toml:"substitute_exclude"`

	// Vars pins variable values per invocation. It is deliberately not
	// loadable from .overlay.toml: a committed pin would permanently shadow
	// the ambient environment that substitution exists to consume.
	Vars []string `toml:"-" config:"vars" pflag_singular:"var" help:"pin NAME=value; --vars takes comma-separated pairs"`
}

// RenderStrategy is the user-configured rendering behavior for one target.
type RenderStrategy string

const (
	// RenderStrategyAppend appends active layers in Overlay layer order.
	RenderStrategyAppend RenderStrategy = "append"
	// RenderStrategyCopy copies the highest-precedence active layer.
	RenderStrategyCopy RenderStrategy = "copy"
)

// RenderRule configures rendering behavior for one target-relative path.
type RenderRule struct {
	Path     string         `toml:"path"`
	Strategy RenderStrategy `toml:"strategy"`
}

// Default returns a Config populated with the default raw values.
func Default() Config {
	return Config{
		Sources:     []string{"."},
		DotPrefix:   true,
		Profiles:    []string{},
		EnvProfiles: []string{},
		Ignore:      []string{},
		RenderRules: []RenderRule{},
	}
}
