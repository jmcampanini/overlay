package cli

import (
	"fmt"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/render"
	"github.com/spf13/cobra"
)

type diffOptions struct {
	configPath string
	target     string
}

func diffCommand() *cobra.Command {
	var opts diffOptions

	cmd := &cobra.Command{
		Use:   "diff [--target DIR]",
		Short: "Print a patch from the target directory to the rendered files",
		Long: `Print on stdout a unified patch that turns the files in the target
directory into what 'overlay render' would write. The configuration,
layers, --target rules, and rendering rules are those of render; see
'overlay render --help'. Nothing is written.

Each new or changed file is a 'diff --git a/PATH b/PATH' block with PATH
relative to the target, so from the target directory the output feeds
'git apply' directly, and it pipes unchanged into any pager or diff
viewer. A PATH holding a space, quote, backslash, or control character is
double-quoted with C escapes, as git writes it. A new file diffs against
/dev/null with 'new file mode' 100644, or 100755 when the source file is
executable; a new empty file has only those header lines. A symbolic link
at a target path appears as git shows a type change: a 'deleted file mode
120000' block holding the link destination, then the new file block.
'patch -p1' applies the ordinary file blocks; quoted paths, the link type
change, and the header-only empty file need a patch program that reads
git's extensions, which the patch shipped with macOS does not. A file
render would skip prints nothing, and when nothing would change stdout is
empty. The exit status is 0 whether or not anything differs; failures
exit 1 as they do for render. Output is never colored.

Every file is treated as text, and a template contributes its rendered
output, so anything a template reads, including a secret, appears in the
patch. A file whose rendered bytes match the target but whose permission
bits differ cannot be expressed as a patch; it is reported on stderr as
'overlay: PATH: no content change, but render would set permission bits
OLD -> NEW' instead. A target path holding anything other than a regular
file, a symbolic link, or nothing is an error.

` + configFileHelp + `
--config PATH reads PATH instead and fails when PATH does not exist.

` + layerSelectionHelp,
		Example: `  overlay diff
  overlay diff | delta
  cd ~ && overlay diff | git apply
  overlay diff --target ~/staging --layers personal`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiff(cmd, opts)
		},
	}
	if err := config.RegisterFlags(cmd.Flags()); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "directory to compare against (default the home directory)")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "configuration file to read instead of the default")
	return cmd
}

func runDiff(cmd *cobra.Command, opts diffOptions) error {
	plan, home, err := loadPlan(cmd, opts.configPath, opts.target)
	if err != nil {
		return err
	}

	omissions, err := render.WritePatch(cmd.OutOrStdout(), plan)
	if err != nil {
		return err
	}
	for _, omission := range omissions {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "overlay: %s: no content change, but render would %s\n",
			collapseHome(omission.Entry.Target, home), omission.Reason); err != nil {
			return err
		}
	}
	return nil
}
