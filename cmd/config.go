package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func newConfigCmd(globalFlags *globalFlags) *cobra.Command {
	var validatePath string
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show loaded configuration, provenance, and effective runtime values.",
		Long: `Show loaded configuration after applying defaults, config file,
environment variables, and config-backed flags. The report uses GoConfigLoader
provenance, then adds Overlay runtime-derived comments such as effective
profiles and expanded paths.

With --validate <path>, parse the given file, merge environment variables and
config-backed flags, and validate the effective runtime configuration. Exits 0
on success, 1 on any error.

For the full schema reference with field descriptions, run: overlay docs`,
		RunE: func(command *cobra.Command, _ []string) error {
			if validatePath != "" {
				return validateConfig(command, validatePath)
			}
			return printLoadedConfig(command, globalFlags, os.Stdout)
		},
	}
	cmd.Flags().StringVar(&validatePath, "validate", "", "validate the given .overlay.toml as effective runtime config and exit")
	return cmd
}
