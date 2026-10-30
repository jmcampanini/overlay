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
	configPath string
	dryRun     bool
	jsonOutput bool
	target     string
}

func renderCommand(options *commandOptions) *cobra.Command {
	var opts renderOptions

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

  env NAME         the environment variable's value; an unset variable
                   fails the render, and a variable set to "" is set
  envOr NAME DEF   the environment variable's value, or DEF when unset
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

A file named NAME.olay.LAYER.EXT is a fragment of NAME.EXT, and a file
named NAME.olay.LAYER is a fragment of NAME. Every fragment for one target
composes into that one file in layer order. When the target's last
extension is json, toml, yaml, or yml, the fragments parse as that format
and merge into one document; with any other extension, or none, the
fragments concatenate as text. The olay token is only a marker. LAYER is
the layer the fragment belongs to, whatever layer its package is listed
under, and a fragment whose LAYER is not in the layer order or not
declared in that source's manifest is skipped. NAME and EXT may hold
dots, LAYER follows the layer grammar with base and local allowed, and
any other shape is an error, as is a merge format spelled in another case
such as JSON. So dot-codex/config.olay.base.toml in a base package and
dot-codex/config.olay.personal.toml in a personal package merge into
~/.codex/config.toml, and dot-zshrc.olay.base beside
dot-zshrc.olay.personal concatenate into ~/.zshrc. NAME.yaml and NAME.yml
are different targets, so every fragment of one file spells the extension
the same way; a fragment of NAME.yaml beside any NAME.yml, or the
reverse, fails the render.

A fragment whose name ends in .tmpl, such as NAME.olay.LAYER.EXT.tmpl or
NAME.olay.LAYER.tmpl, is a template of a fragment: it executes as a
text/template with the data and functions above, and its output merges or
concatenates like any other fragment of the target. The suffix is only a
marker, goes last, and is stripped once; a .tmpl before the olay marker
is part of NAME. Every templated fragment of a target renders before any
fragment is composed, so a template error names the fragment that failed
rather than the parse error it would cause. A fragment whose output is
empty or only whitespace is an empty document in every merge format and
adds nothing to a concatenation, so a fragment wrapped in
{{ if eq .OS "darwin" }} ... {{ end }} contributes nothing elsewhere.
Templated and plain fragments mix freely across layers; two for one target
in one layer collide like any other pair.

Merged fragments combine key by key: tables, objects, and mappings merge
recursively, arrays and sequences concatenate with the later layer's items
last and nothing removed, and any other value takes the later layer's. A
table, an array, and a plain value meeting at one key is an error naming
both fragments. Nothing deletes a key or replaces a subtree. The output
sorts keys alphabetically at every level, drops comments, indents JSON
with two spaces, writes TOML tables flush-left, writes YAML in block style
with two-space indentation, and ends with a newline unless the document
is empty TOML; a target with one fragment is rewritten the same way.

Concatenated fragments are text joined in layer order with the base
layer's fragment first. Each fragment that is not empty or only
whitespace is written as it is, followed by a newline when it does not
end in one, and nothing else is added, so no header or blank line marks
where one fragment ends. A target with one fragment is that fragment,
plus the newline if it lacked one. Concatenation knows nothing of the
file's syntax, so a later layer can only append; it cannot change a line
an earlier layer wrote. A fragment holding a NUL byte is binary and fails
the render.

However fragments compose, a plain NAME.EXT or NAME.EXT.tmpl in a later
layer replaces the composed file whole, and a fragment in a later layer
than a plain NAME.EXT is an error. Two fragments for one target with the
same LAYER fail like any same-layer collision. The target keeps only the
permission bits every fragment has, so one 600 fragment in the stack
denies group and other access to the whole target.

YAML fragments keep numbers and timestamps as written, so 1.10 stays
1.10, expand aliases in place, read an empty or comment-only file as an
empty mapping, and double-quote strings that would otherwise read as
another type, including the YAML 1.1 words yes, no, on, and off. A merge
key (<<), a key that is not a string, a duplicate key, a tag outside the
core schema such as !!binary, or a second document in one fragment is an
error.

Fragment text is otherwise taken as written; an environment variable
reaches a fragment only through a templated fragment and env or envOr. A
fragment that does not parse, whose JSON root is not an object, or whose
YAML root is not a mapping, or a concatenated fragment holding a NUL
byte, fails the whole render, with --dry-run too, before anything is
written; a fragment a later plain NAME.EXT replaces is never read.

For example, this loader sources every envs/env.LAYER that exists, in
layer order and then source order:

  {{ range $layer := .Layers }}{{ range $.Sources }}
  {{- $f := printf "%s/envs/env.%s" .Path $layer }}{{ if fileExists $f }}
  test -f {{ shellQuote $f }} && source {{ shellQuote $f }}
  {{ end }}{{ end }}{{ end }}

Writing creates missing directories with mode 755, writes each file to a
temporary .NAME.overlay-* in its final directory with the source's
permission bits, and renames it over the target, which discards the
target's previous content; a symbolic link at the target is replaced
rather than followed, and a failed write removes its temporary file. No
other file is ever deleted. A file is skipped when the target is a regular
file with identical bytes and permission bits. A directory at a target
path is an error. Paths that escape the target, including through parent
symbolic links, and paths that replace overlay's active configuration are
rejected before writing. A failure while writing stops at that file, and
files already written stay in place.

Both modes print the plan on stdout: a table with columns TARGET, SOURCE
(name:path within the source; a merge or concat lists every fragment in
layer order), KIND (file, template, merge, or concat), STATUS (new,
changed, or unchanged; a template, merge, or concat compares its rendered
output), and REPLACES (the files earlier layers contributed), then a
count summary.
Targets under the home directory are shortened to ~. --color MODE is auto
by default: the table colors STATUS only when stdout is a terminal whose
TERM is not dumb, and a nonempty NO_COLOR disables color. always and never
override both. --json instead emits one JSON document {"target",
"layers", "entries": [{"target", "source", "path", "layer", "kind",
"status", "replaces": [{"source", "path", "layer"}], "fragments":
[{"source", "path", "layer"}]}]} with absolute paths and no escapes, even
with --color always. For a merge or concat, fragments lists its fragments
in layer order and source, path, and layer name the last of them; for
other kinds fragments is empty.

` + contractHelp + `

` + configFileHelp + `
--config PATH reads PATH instead and fails when PATH is empty or missing.

` + layerSelectionHelp,
		Example: `  overlay render --dry-run
  overlay render --target ~/tmp/home --layers personal
  overlay render --dry-run --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRender(cmd, opts, options.color)
		},
	}
	if err := config.RegisterFlags(cmd.Flags()); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "directory to render into (default the home directory)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the plan without writing")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print the plan as JSON")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "configuration file to read instead of the default")
	return cmd
}

func runRender(cmd *cobra.Command, opts renderOptions, color colorMode) error {
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
	profile := resolveColor(color, os.Getenv("NO_COLOR") != "", detectColor(out, os.Environ()))
	return writePlanTable(out, plan, home, colorEnabled(profile))
}

// loadPlan loads the configuration the command's flags and configPath select
// and plans a render into the resolved target. It returns the plan and the
// home directory the target was resolved against.
func loadPlan(cmd *cobra.Command, configPath, target string) (render.Plan, string, error) {
	env := configloader.OSEnv()
	cfg, report, err := config.Load(config.LoadOptions{
		Env:     env,
		Flags:   cmd.Flags(),
		Path:    configPath,
		PathSet: cmd.Flags().Changed("config"),
	})
	if err != nil {
		return render.Plan{}, "", err
	}
	effective, err := config.Resolve(cfg, env)
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
		Home:           home,
		Hostname:       os.Hostname,
		Layers:         config.LayerOrder(effective.Layers),
		OS:             runtime.GOOS,
		ProtectedPaths: report.LoadedFiles,
		Sources:        effective.Sources,
		Target:         absTarget,
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
