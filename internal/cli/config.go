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

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
)

var configValidate string

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the fully-resolved configuration, or validate a file.",
		Long: `Show the fully-resolved configuration after applying the precedence rules
for flags, config file, and environment variables. Each field is annotated
with "# from: ..." indicating its source (flag, config, env, or default).

With --validate <path>, parse and schema-check the given file without
merging any flags or env vars. Exits 0 on success, 1 on any error.

For the full schema reference with field descriptions, run: overlay docs`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if configValidate != "" {
				return config.Validate(configValidate)
			}
			r, err := Resolve(cmd, &globals)
			if err != nil {
				return err
			}
			return printResolved(os.Stdout, r)
		},
	}
	cmd.Flags().StringVar(&configValidate, "validate", "", "validate the given .overlay.toml file and exit")
	return cmd
}

// printResolved writes the resolved Settings as TOML-like output with
// `# from:` trailing comments on every field. Output is tab-aligned so
// the "from" column is easy to scan.
func printResolved(w io.Writer, r Resolved) error {
	notFound := ""
	if !r.ConfigExists {
		notFound = " (not found)"
	}
	if _, err := fmt.Fprintf(w, "# overlay configuration (resolved)\n# config file: %s%s\n\n", r.ConfigPath, notFound); err != nil {
		return err
	}

	s := r.Settings
	lines := []struct {
		key, value, from string
	}{
		{"source", fmt.Sprintf("%q", s.SourceDir), r.SourceFrom},
		{"target", fmt.Sprintf("%q", s.TargetDir), r.TargetFrom},
		{"dot_prefix", fmt.Sprintf("%t", s.DotPrefix), r.DotPrefixFrom},
		{"profiles", fmt.Sprintf("[%s]", quoteList(s.Profiles)), r.ProfilesFrom.String()},
		{"continue_on_error", fmt.Sprintf("%t", r.ContinueOnError), r.ContinueFrom},
		{"traverse_hidden", fmt.Sprintf("%t", s.TraverseHidden), r.TraverseHiddenFrom},
		{"respect_gitignore", fmt.Sprintf("%t", s.RespectGitignore), r.RespectGitignoreFrom},
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, l := range lines {
		if _, err := fmt.Fprintf(tw, "%s\t= %s\t# from: %s\n", l.key, l.value, l.from); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func quoteList(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	qs := make([]string, len(xs))
	for i, x := range xs {
		qs[i] = fmt.Sprintf("%q", x)
	}
	return strings.Join(qs, ", ")
}
