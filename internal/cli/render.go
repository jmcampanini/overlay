package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jmcampanini/go-config-loader/configloader"
	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/render"
	"github.com/spf13/cobra"
)

type renderOptions struct {
	color      colorMode
	configPath string
	dryRun     bool
	jsonOutput bool
	target     string
}

func renderCommand() *cobra.Command {
	opts := renderOptions{color: colorAuto}

	cmd := &cobra.Command{
		Use:   "render [--target DIR] [--dry-run] [--json] [--color MODE]",
		Short: "Render the selected layers into the target directory",
		Long: `Write the files the effective layers select from every source into the
target directory. --target DIR defaults to the home directory and accepts
an absolute path, a path starting with ~/, or a path relative to the
current directory; it must already exist. --dry-run plans and prints
without writing.

Each source keeps an overlay.toml at its root with one [layers.NAME] table
per layer, whose packages list names top-level directories of the source:

  [layers.base]
  packages = ["bash", "ssh"]

A listed package must exist, layer names follow the grammar in 'overlay
config --help' with base and local also allowed, and unknown keys are
errors. A layer no manifest mentions contributes nothing. Layers apply in
the order 'overlay config' reports as layer_order, and sources in file
order within a layer.

Every regular file in a selected package renders; a symbolic link to a
regular file is followed and its content copied, and any other entry is an
error. The target path is the file's path inside the package with every
path segment that starts with dot- given a leading dot instead, so
bash/dot-bashrc becomes ~/.bashrc. A file from a later layer replaces an
earlier layer's file at the same target. Two files in one layer with the
same target, even from one source, fail the whole render before anything
is written.

A file whose name ends in .tmpl is a Go text/template and renders to the
target path without that suffix, so envs/dot-shellenv.tmpl becomes
~/.shellenv with the template's permission bits. A name that is only the
suffix is an error. Templates execute while planning, so a template that
fails to parse or execute fails the whole render, with --dry-run too,
before anything is written. A missing field or map key is an error rather
than empty output. Every template sees the same data:

  .Layers    the layer order as a list, including base and local
  .Sources   the configured sources in order; each has .Name and .Path,
             the absolute checkout directory
  .Home      the home directory, whatever --target is
  .OS        the operating system as Go names it: darwin or linux
  .Hostname  the hostname exactly as the system reports it

Beyond the text/template builtins, templates may call:

  env NAME         the environment variable's value, or "" when unset
  fileExists PATH  true when anything exists at PATH (a file, directory,
                   or other entry, following symbolic links); PATH is used
                   as given, ~ is not expanded, and an error other than
                   absence fails the render
  hasLayer NAME    true when NAME is one of the layers in .Layers
  readFile PATH    the file's content with trailing newlines removed, as
                   $(cat PATH) would give; PATH is used as given, and a
                   missing or unreadable file fails the render, so guard
                   an optional file with fileExists
  shellQuote S     S single-quoted so bash, zsh, and fish read one literal
                   word; quotes and backslashes inside S are escaped
                   outside the quotes
  join SEP LIST    LIST's items concatenated with SEP between them

A secret can stay in a file outside every source and be read at render
time, for example {{ readFile (printf "%s/.config/overlay/local/api.key"
.Home) }}. The plan output never prints rendered content, and the target
takes the template's permission bits, so keep such a template at 600.

A file named NAME.olay.LAYER.EXT is a fragment of NAME.EXT. With EXT json
or toml, every fragment for one target parses as that format and the
fragments merge in layer order into one document written to NAME.EXT. The
olay token is only a marker. LAYER is the layer the fragment belongs to,
whatever layer its package is listed under, and a fragment whose LAYER is
not in the layer order is skipped. NAME may hold dots, EXT is one
segment, LAYER follows the layer grammar with base and local allowed, and
any other shape is an error. So dot-codex/config.olay.base.toml in a base
package and dot-codex/config.olay.personal.toml in a personal package
merge into ~/.codex/config.toml.

Fragments merge key by key: tables and objects merge recursively, arrays
concatenate with the later layer's items last and nothing removed, and
any other value takes the later layer's. A table, an array, and a plain
value meeting at one key is an error naming both fragments. Nothing
deletes a key or replaces a subtree; a plain NAME.EXT or NAME.EXT.tmpl in
a later layer replaces the merged document whole, and a fragment in a
later layer than a plain NAME.EXT is an error. Two fragments for one
target with the same LAYER fail like any same-layer collision. The output
sorts keys alphabetically at every level, drops comments, indents JSON
with two spaces, writes TOML tables flush-left, and ends with a newline
unless the document is empty TOML; a target with one fragment is
rewritten the same way. The target takes the last fragment's permission
bits.

Before merging, every string value and key in a fragment has ${NAME}
replaced by the environment variable NAME, where NAME is letters, digits,
and underscores not starting with a digit. An unset variable fails the
render, $${ writes a literal ${, and any other text is left as written.
Values stay strings, so a number or boolean cannot come from a variable.
A fragment that does not parse, whose JSON root is not an object, or
whose keys collide after substitution fails the whole render, with
--dry-run too, before anything is written; a fragment a later plain
NAME.EXT replaces is never read.

For example, this loader sources every envs/env.LAYER that exists, in
layer order and then source order:

  {{ range $layer := .Layers }}{{ range $.Sources }}
  {{- $f := printf "%s/envs/env.%s" .Path $layer }}{{ if fileExists $f }}
  test -f {{ shellQuote $f }} && source {{ shellQuote $f }}
  {{ end }}{{ end }}{{ end }}

Writing creates missing directories with mode 755, writes each file to a
temporary name in its final directory with the source's permission bits,
and renames it over the target, so a symbolic link at the target is
replaced rather than followed. Nothing is ever deleted. A file is skipped
when the target is a regular file with identical bytes and permission
bits. A directory at a target path is an error. A failure while writing
stops at that file, and files already written stay in place.

Both modes print the plan on stdout: a table with columns TARGET, SOURCE
(name:path within the source; a merge lists every fragment in layer
order), KIND (file, template, or merge), STATUS (new, changed, or
unchanged; a template or merge compares its rendered output), and
REPLACES (the files earlier layers contributed), then a count summary.
Targets under the home directory are shortened to ~. The table colors
STATUS only when stdout is a terminal whose TERM is not dumb; a nonempty
NO_COLOR disables color and --color MODE (auto, always, or never)
overrides both. --json instead emits one JSON document {"target",
"layers", "entries": [{"target", "source", "path", "layer", "kind",
"status", "replaces": [{"source", "path", "layer"}], "merges": [{"source",
"path", "layer"}]}]} with absolute paths and no escapes, even with --color
always. For a merge, merges lists its fragments in layer order and
source, path, and layer name the last of them; for other kinds merges is
empty.

` + configFileHelp + `
--config PATH reads PATH instead and fails when PATH does not exist.

` + layerSelectionHelp,
		Example: `  overlay render --dry-run
  overlay render --target ~/tmp/home --layers personal
  overlay render --dry-run --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRender(cmd, opts)
		},
	}
	if err := config.RegisterFlags(cmd.Flags()); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "directory to render into (default the home directory)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the plan without writing")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print the plan as JSON")
	cmd.Flags().Var(colorValue{mode: &opts.color}, "color", "color the table: auto, always, or never")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "configuration file to read instead of the default")
	return cmd
}

func runRender(cmd *cobra.Command, opts renderOptions) error {
	plan, home, err := loadPlan(cmd, opts.configPath, opts.target)
	if err != nil {
		return err
	}
	if !opts.dryRun {
		if err := render.Apply(plan); err != nil {
			return err
		}
	}

	out := cmd.OutOrStdout()
	if opts.jsonOutput {
		return writePlanJSON(out, plan)
	}
	profile := resolveColor(opts.color, cmd.Flags().Changed("color"),
		os.Getenv("NO_COLOR") != "", detectColor(out, os.Environ()))
	return writePlanTable(out, plan, home, colorEnabled(profile))
}

// loadPlan loads the configuration the command's flags and configPath select
// and plans a render into the resolved target. It returns the plan and the
// home directory the target was resolved against.
func loadPlan(cmd *cobra.Command, configPath, target string) (render.Plan, string, error) {
	cfg, _, err := config.Load(config.LoadOptions{
		Env:   configloader.OSEnv(),
		Flags: cmd.Flags(),
		Path:  configPath,
	})
	if err != nil {
		return render.Plan{}, "", err
	}
	effective, err := config.Resolve(cfg)
	if err != nil {
		return render.Plan{}, "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return render.Plan{}, "", fmt.Errorf("locate home directory: %w", err)
	}
	absTarget, err := resolveTarget(target, home)
	if err != nil {
		return render.Plan{}, "", err
	}

	plan, err := render.Build(render.Request{
		Home:     home,
		Hostname: os.Hostname,
		Layers:   config.LayerOrder(effective.Layers),
		OS:       runtime.GOOS,
		Sources:  effective.Sources,
		Target:   absTarget,
	})
	if err != nil {
		return render.Plan{}, "", err
	}
	return plan, home, nil
}

// resolveTarget turns the --target value into an absolute path: empty means
// home, ~ and ~/ expand to home, and a relative path resolves against the
// current directory.
func resolveTarget(target, home string) (string, error) {
	switch {
	case target == "":
		return home, nil
	case target == "~" || strings.HasPrefix(target, "~/"):
		return filepath.Clean(filepath.Join(home, strings.TrimPrefix(target, "~"))), nil
	case strings.HasPrefix(target, "~"):
		return "", fmt.Errorf("target %q: only ~ and ~/ expand; ~user is not supported", target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve target %q: %w", target, err)
	}
	return abs, nil
}
