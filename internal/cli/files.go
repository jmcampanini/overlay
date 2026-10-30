package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/spf13/cobra"
)

func filesCommand() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "files PATTERN...",
		Short: "List source file paths in layer order",
		Long: `Print one absolute, cleaned path per line on stdout, including a newline
after the last path. Symbolic links are listed by their own paths, without
resolving them. A path containing a newline fails before any output.
File contents are not read. No color or other formatting is added. The
files template function 'overlay help templates' describes selects the
same paths with the same rules.

` + fileSelectionHelp + `

` + contractHelp + `

` + configFileHelp + `
` + configFlagHelp + `

` + layerSelectionHelp,
		Example: `  overlay files 'packages/pacman/packages.{layer}'
  overlay files --layers work,laptop 'packages/pacman/packages.{layer}'
  overlay files 'packages/homebrew/Brewfile.{layer}' | tail -n 1`,
		Args: filePatternArgs,
		RunE: func(cmd *cobra.Command, patterns []string) error {
			paths, err := selectFiles(cmd, configPath, patterns)
			if err != nil {
				return err
			}
			var output strings.Builder
			for _, path := range paths {
				if strings.Contains(path, "\n") {
					return fmt.Errorf("path %q contains a newline", path)
				}
				output.WriteString(path)
				output.WriteByte('\n')
			}
			_, err = io.WriteString(cmd.OutOrStdout(), output.String())
			return err
		},
	}
	if err := config.RegisterFlags(cmd.Flags()); err != nil {
		panic(err)
	}
	registerConfigFlag(cmd, &configPath)
	return cmd
}
