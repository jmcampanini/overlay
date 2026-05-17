// Package cli wires the cobra commands for the overlay binary and
// resolves config + env + flags into the Settings consumed by the
// render, diff, and plan packages.
package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/jmcampanini/go-config-loader/configreporter"
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
)

var configValidate string

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the raw loaded configuration, or validate a file.",
		Long: `Show the raw loaded configuration after applying defaults, config file,
environment variables, and config-backed flags. The report uses GoConfigLoader
provenance and does not include Overlay runtime-derived values such as effective
profiles or expanded paths.

With --validate <path>, parse and schema-check the given file without
merging any flags or env vars. Exits 0 on success, 1 on any error.

For the full schema reference with field descriptions, run: overlay docs`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if configValidate != "" {
				return config.ValidateFile(configValidate)
			}
			raw, err := loadRawConfig(cmd, &globals)
			if err != nil {
				return err
			}
			return printRawConfig(os.Stdout, raw)
		},
	}
	cmd.Flags().StringVar(&configValidate, "validate", "", "validate the given .overlay.toml file and exit")
	return cmd
}

func printRawConfig(w io.Writer, raw rawLoadedConfig) error {
	notFound := ""
	if len(raw.Report.LoadedFiles) == 0 {
		notFound = " (not found)"
	}
	if _, err := fmt.Fprintf(w, "# overlay configuration (raw)\n# config file: %s%s\n\n", raw.ConfigPath, notFound); err != nil {
		return err
	}

	reporter := configreporter.New(raw.Config, raw.Report)
	if err := reporter.WriteTOML(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n# provenance"); err != nil {
		return err
	}

	headers := reporter.ProvenanceHeaders()
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "# %s\t%s\n", headers[0], headers[1]); err != nil {
		return err
	}
	for _, row := range reporter.ProvenanceRows() {
		if _, err := fmt.Fprintf(tw, "# %s\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	return tw.Flush()
}
