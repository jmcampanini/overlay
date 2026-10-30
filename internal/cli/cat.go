package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/spf13/cobra"
)

func catCommand() *cobra.Command {
	var configPath string
	var list bool
	cmd := &cobra.Command{
		Use:   "cat [--list] PATTERN...",
		Short: "Print source file contents in layer order",
		Long: `Read every matched file before writing anything to stdout. Raw mode
prints each file's bytes unchanged, adding one newline after each nonempty
file that lacks a final newline. Empty files add nothing. A .tmpl file
prints as stored, with its template text unrendered. There are no headers,
separators, or markers, and no color or other formatting is added.

--list treats each file as one entry per line, split on newline. It trims
leading and trailing spaces, tabs, and carriage returns, then drops empty
lines and lines starting with #. A # preceded by a space or tab starts a
trailing comment: cut from that # onward, trim again, and drop an empty
result. A # inside a word or URL stays intact. Internal whitespace stays
as written. Entries are compared as exact bytes after trimming; duplicates
across all files are dropped, keeping the first position. Each entry is
printed with a final newline. Raw mode never drops or deduplicates lines.
An unreadable file fails before any output.

` + fileSelectionHelp + `

` + contractHelp + `

` + configFileHelp + `
--config PATH reads PATH instead and fails when PATH is empty or missing.

` + layerSelectionHelp,
		Example: `  overlay cat --list 'packages/pacman/packages.{layer}' |
    xargs -r -o sudo pacman -Syu --needed
  overlay cat 'packages/homebrew/Brewfile.{layer}' | brew bundle --file=-
  overlay cat --list --layers work,laptop 'packages/pacman/packages.{layer}'`,
		Args: filePatternArgs,
		RunE: func(cmd *cobra.Command, patterns []string) error {
			paths, err := selectFiles(cmd, configPath, patterns)
			if err != nil {
				return err
			}
			var output bytes.Buffer
			seen := make(map[string]bool)
			for _, path := range paths {
				content, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read file %q: %w", path, err)
				}
				if list {
					appendList(&output, seen, content)
					continue
				}
				output.Write(content)
				if len(content) > 0 && content[len(content)-1] != '\n' {
					output.WriteByte('\n')
				}
			}
			_, err = output.WriteTo(cmd.OutOrStdout())
			return err
		},
	}
	if err := config.RegisterFlags(cmd.Flags()); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&configPath, "config", "", "configuration file to read instead of the default")
	cmd.Flags().BoolVar(&list, "list", false, "trim whitespace and comments; print unique entries")
	return cmd
}

func appendList(output *bytes.Buffer, seen map[string]bool, content []byte) {
	for _, line := range strings.Split(string(content), "\n") {
		entry := strings.Trim(line, " \t\r")
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		entry, _, _ = strings.Cut(entry, " #")
		entry, _, _ = strings.Cut(entry, "\t#")
		entry = strings.Trim(entry, " \t\r")
		if seen[entry] {
			continue
		}
		seen[entry] = true
		output.WriteString(entry)
		output.WriteByte('\n')
	}
}
