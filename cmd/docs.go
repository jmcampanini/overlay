package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
)

func newDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print the full .overlay.toml schema reference.",
		Long: `Print the complete reference for the .overlay.toml file format: every
field, its type, default, and description. Pipeable to less or bat.

The reference also covers variable substitution, the config-backed
environment variables, source and profile resolution precedence, and the
file convention. It is plain text on stdout with no terminal escapes, and
no configuration is read. Command help (--help on each command and
'overlay help exit-codes') is the canonical contract; docs supplements it
with the longer reference.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(os.Stdout, config.SchemaDocs)
			return err
		},
	}
}
