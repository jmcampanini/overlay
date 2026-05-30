// Package cli wires the cobra commands for the overlay binary and
// resolves config + env + flags into the Settings consumed by the
// render, diff, and plan packages.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jmcampanini/go-config-loader/configreporter"
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
)

var configValidate string

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show loaded configuration, provenance, and effective runtime values.",
		Long: `Show loaded configuration after applying defaults, config file,
environment variables, and config-backed flags. The report uses GoConfigLoader
provenance, then adds Overlay runtime-derived comments such as effective
profiles and expanded paths.

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
			return printConfig(os.Stdout, raw)
		},
	}
	cmd.Flags().StringVar(&configValidate, "validate", "", "validate the given .overlay.toml file and exit")
	return cmd
}

func printConfig(w io.Writer, raw rawLoadedConfig) error {
	effective := deriveConfigEffective(raw)
	effectiveErrors := validateConfigEffective(raw, effective)

	notFound := ""
	if len(raw.Report.LoadedFiles) == 0 {
		notFound = " (not found)"
	}
	if _, err := fmt.Fprintf(w, "# overlay configuration\n# config file: %s%s\n\n", raw.ConfigPath, notFound); err != nil {
		return err
	}

	reporter := configreporter.New(raw.Config, raw.Report)
	if err := reporter.WriteTOML(w); err != nil {
		return err
	}
	if err := writeConfigProvenance(w, reporter, raw.Report.LoadedFiles); err != nil {
		return err
	}
	return writeConfigEffective(w, effective, effectiveErrors)
}

func writeConfigProvenance(w io.Writer, reporter configreporter.Reporter[config.Config], loadedFiles []string) error {
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
	if err := tw.Flush(); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "# loaded_files = [%s]\n", quoteList(loadedFiles)); err != nil {
		return err
	}
	return nil
}

func writeConfigEffective(w io.Writer, effective configEffective, effectiveErrors []configEffectiveError) error {
	if _, err := fmt.Fprintln(w, "\n# effective:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "# effective_source_dirs = [%s]\n", quoteList(effective.SourceDirs)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "# effective_target_dir = %q\n", effective.TargetDir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "# effective_profiles = [%s]\n", quoteList(effective.Profiles)); err != nil {
		return err
	}
	if len(effectiveErrors) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "# effective_errors:"); err != nil {
		return err
	}
	for _, effectiveErr := range effectiveErrors {
		if _, err := fmt.Fprintf(w, "# %s = %q\n", effectiveErr.Field, effectiveErr.Err.Error()); err != nil {
			return err
		}
	}
	return nil
}

func quoteList(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}
