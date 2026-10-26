package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/jmcampanini/go-config-loader/configloader"
	"github.com/jmcampanini/go-config-loader/configreporter"
	"github.com/jmcampanini/overlay/internal/config"
	"github.com/spf13/cobra"
)

func configCommand() *cobra.Command {
	var configPath string
	var showProvenance bool

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the configuration in effect",
		Long: `Print the configuration in effect as TOML on stdout, followed by commented
effective values. Nothing is modified and nothing is redacted; the file
holds no secrets.

` + configFileHelp + `
--config PATH reads PATH instead and fails when PATH does not exist or
is empty.

Settings load from built-in defaults (an empty layer list and no sources),
the configuration file, OVERLAY_* environment variables, then flags, each
overriding the one before. Only layers has an environment variable,
OVERLAY_LAYERS, and flags, --layers and --layer; env_layers and sources
are read from the file only.

` + layerSelectionHelp + `

File fields: layers (list), env_layers (list of environment variable
names; a blank or whitespace-padded name is an error), and [[source]]
entries with url (required), path (absolute or starting with ~/,
required), and name (optional). A source's name defaults to the last
segment of its url with any .git suffix removed; two sources resolving to
the same name is an error, fixed by setting name on one of them. Two
sources sharing a path is also an error. Source order is the tie-break
order for later commands.

The TOML body reloads as config.toml and shows layers and env_layers as
loaded, before the env_layers variables are read. The '# Effective'
comment block lists loaded_files, effective_layers after the env_layers
additions and the last-occurrence rule, layer_order (base, the effective
layers, then local), and each source's name with its absolute path.
--provenance appends a '# Provenance' table naming where each field came
from: <default>, the file path, <env>, or <pflag>.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := configloader.OSEnv()
			cfg, report, err := config.Load(config.LoadOptions{
				Env:   env,
				Flags: cmd.Flags(),
				Path:  configPath,
			})
			if err != nil {
				return err
			}
			effective, err := config.Resolve(cfg, env)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			reporter := configreporter.New(cfg, report)
			if err := reporter.WriteTOML(out); err != nil {
				return err
			}
			if err := writeEffective(out, report.LoadedFiles, effective); err != nil {
				return err
			}
			if !showProvenance {
				return nil
			}
			return writeProvenance(out, reporter)
		},
	}
	if err := config.RegisterFlags(cmd.Flags()); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&configPath, "config", "", "configuration file to read instead of the default")
	cmd.Flags().BoolVar(&showProvenance, "provenance", false, "append the source of each field")
	return cmd
}

func writeEffective(out io.Writer, loadedFiles []string, effective config.Effective) error {
	var lines strings.Builder
	lines.WriteString("\n# Effective\n")
	fmt.Fprintf(&lines, "# loaded_files = [%s]\n", quotedList(loadedFiles))
	fmt.Fprintf(&lines, "# effective_layers = [%s]\n", quotedList(effective.Layers))
	fmt.Fprintf(&lines, "# layer_order = [%s]\n", quotedList(config.LayerOrder(effective.Layers)))
	for _, source := range effective.Sources {
		fmt.Fprintf(&lines, "# source %s = %q\n", source.Name, source.Path)
	}

	_, err := io.WriteString(out, lines.String())
	return err
}

func writeProvenance(out io.Writer, reporter configreporter.Reporter[config.Config]) error {
	if _, err := fmt.Fprintln(out, "\n# Provenance"); err != nil {
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	headers := reporter.ProvenanceHeaders()
	if _, err := fmt.Fprintf(w, "# %s\t%s\t%s\n", headers[0], headers[1], headers[2]); err != nil {
		return err
	}
	for _, row := range reporter.ProvenanceRows() {
		if _, err := fmt.Fprintf(w, "# %s\t%s\t%s\n", row[0], row[1], row[2]); err != nil {
			return err
		}
	}
	return w.Flush()
}

func quotedList(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}
