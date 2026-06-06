// Package cli contains Cobra-coupled configuration resolution helpers for the command package.
package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/jmcampanini/go-config-loader/configreporter"
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
)

// TODO: split config reporting and validation out of this Cobra-coupled helper package.

// PrintConfig loads runtime configuration and writes the config report.
func PrintConfig(cmd *cobra.Command, g *GlobalFlags, w io.Writer) error {
	raw, err := loadRawConfig(cmd, g)
	if err != nil {
		return err
	}
	return printConfig(w, raw)
}

// ValidateConfig validates path as an effective runtime configuration.
func ValidateConfig(cmd *cobra.Command, path string) error {
	return runConfigValidate(cmd, path)
}

func runConfigValidate(cmd *cobra.Command, path string) error {
	raw, err := loadRawConfigFromPath(cmd, path, true)
	if err != nil {
		return err
	}
	effective := deriveEffectiveConfig(raw)
	return effectiveConfigErrors(validateEffectiveConfig(raw, effective)).Err()
}

func printConfig(w io.Writer, raw rawLoadedConfig) error {
	effective := deriveEffectiveConfig(raw)
	effectiveErrors := validateEffectiveConfig(raw, effective)

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
	return writeEffectiveConfig(w, effective, effectiveErrors)
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

func writeEffectiveConfig(w io.Writer, effective effectiveConfig, effectiveErrors []effectiveConfigError) error {
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
