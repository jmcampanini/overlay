package cli

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"text/tabwriter"

	"github.com/BurntSushi/toml"
	"github.com/jmcampanini/overlay/internal/merge"
	"github.com/spf13/cobra"
)

func valuesCommand() *cobra.Command {
	var paths []string
	var showProvenance bool

	cmd := &cobra.Command{
		Use:   "values [--values PATH]... [--provenance]",
		Short: "Print the merged values templates read as .Values",
		Long: `Print the values document as TOML on stdout, followed by a commented list
of the files it merged. Nothing is modified and nothing is redacted, so a
secret in a values file prints. No configuration file is read.

` + valuesHelp + `

The TOML body is the merged document with keys sorted at every level, and
it reloads as one values file. The '# Effective' comment block lists
values_dir, the directory searched, and loaded_files, every file merged in
order. --provenance appends a '# Provenance' table naming the file that
last contributed to each top-level key, with the key written as TOML
would and the file quoted, so every row stays one comment line.

` + contractHelp,
		Example: `  overlay values
  overlay values --provenance
  overlay values --values ~/themes/gruvbox-dark.toml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			values, err := loadValues(paths)
			if err != nil {
				return err
			}
			body, err := merge.Encode(merge.TOML, values.Document)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if _, err := out.Write(body); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "\n# Effective\n# values_dir = %q\n# loaded_files = [%s]\n", values.Dir, quotedList(values.Files)); err != nil {
				return err
			}
			if !showProvenance {
				return nil
			}
			return writeValuesProvenance(out, values.Origins)
		},
	}
	cmd.Flags().StringArrayVar(&paths, "values", nil, "merge `PATH` after the values directory; repeatable")
	cmd.Flags().BoolVar(&showProvenance, "provenance", false, "append the file that last set each top-level key")
	return cmd
}

func writeValuesProvenance(out io.Writer, origins map[string]string) error {
	if _, err := fmt.Fprintln(out, "\n# Provenance"); err != nil {
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "# key\tfile"); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(origins)) {
		if _, err := fmt.Fprintf(w, "# %s\t%q\n", toml.Key{key}, origins[key]); err != nil {
			return err
		}
	}
	return w.Flush()
}
