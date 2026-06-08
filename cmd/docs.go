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
		Long:  "Print the complete reference for the .overlay.toml file format: every\nfield, its type, default, and description. Pipeable to less or bat.",
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(os.Stdout, config.SchemaDocs)
			return err
		},
	}
}
