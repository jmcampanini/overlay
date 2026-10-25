package cli

import (
	"fmt"
	"os"
	"path/filepath"
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
		Long: `Copy the files the effective layers select from every source into the
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
is written. Files ending in .olay.* are copied verbatim.

Writing creates missing directories with mode 755, writes each file to a
temporary name in its final directory with the source's permission bits,
and renames it over the target, so a symbolic link at the target is
replaced rather than followed. Nothing is ever deleted. A file is skipped
when the target is a regular file with identical bytes and permission
bits. A directory at a target path is an error. A failure while writing
stops at that file, and files already written stay in place.

Both modes print the plan on stdout: a table with columns TARGET, SOURCE
(name:path within the source), KIND (always file in this release), STATUS
(new, changed, or unchanged), and REPLACES (the files earlier layers
contributed), then a count summary. Targets under the home directory are
shortened to ~. The table colors STATUS only when stdout is a terminal
whose TERM is not dumb; a nonempty NO_COLOR disables color and --color
MODE (auto, always, or never) overrides both. --json instead emits one
JSON document {"target", "layers", "entries": [{"target", "source",
"path", "layer", "kind", "status", "replaces": [{"source", "path",
"layer"}]}]} with absolute paths and no escapes, even with --color always.

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
	cfg, _, err := config.Load(config.LoadOptions{
		Env:   configloader.OSEnv(),
		Flags: cmd.Flags(),
		Path:  opts.configPath,
	})
	if err != nil {
		return err
	}
	effective, err := config.Resolve(cfg)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home directory: %w", err)
	}
	target, err := resolveTarget(opts.target, home)
	if err != nil {
		return err
	}

	plan, err := render.Build(render.Request{
		Layers:  config.LayerOrder(effective.Layers),
		Sources: effective.Sources,
		Target:  target,
	})
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
