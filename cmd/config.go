package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func newConfigCmd(flags *globalFlags) *cobra.Command {
	var validatePath string
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show loaded configuration, provenance, and effective runtime values.",
		Long: `Show loaded configuration after applying defaults, config file,
environment variables, and config-backed flags. The report uses
GoConfigLoader provenance, then adds Overlay runtime-derived comments such
as effective profiles and expanded paths.

` + configPrecedenceHelp + `

The report goes to stdout and is valid TOML that reloads as .overlay.toml:
a '# config file:' header naming the file (with '(not found)' when the
default file is absent), the loaded raw values, a '# provenance' table
naming the layer that set each field (<default>, the file path, <env>, or
<pflag>) followed by '# loaded_files', and an '# effective:' block with
effective_source_dirs, effective_target_dir, and effective_profiles after
path expansion and env_profiles. Problems deriving effective values are
listed under '# effective_errors:' and the command still exits 0. Nothing
is redacted: a pin given with --var, --vars, or OVERLAY_VARS appears in
full as NAME=value in the provenance table (and nowhere else), so do not
redirect the report anywhere a pinned secret must not land. Nothing is
written.

sources and target expand a leading ~ and any $VAR or ${VAR} from the
environment; an undefined variable is an error. Relative paths set in the
file resolve from the file's directory, and relative paths set by the
environment or flags resolve from the current directory. The report shows
the loaded strings and comments the expanded effective paths.

` + profilePrecedenceHelp + `

With --validate <path>, parse the given file, merge environment variables and
config-backed flags, and validate the effective runtime configuration. Exits 0
on success, 1 on any error. The file must exist, target must be set by the
file, the environment, or --target, and every problem is reported at once
as 'field: message' lines after 'overlay:' on stderr. --validate ignores
--config and prints nothing on success.

For the full schema reference with field descriptions, run: overlay docs`,
		RunE: func(command *cobra.Command, _ []string) error {
			if validatePath != "" {
				return runConfigValidate(command, validatePath)
			}
			return printLoadedConfig(command, flags, os.Stdout)
		},
	}
	cmd.Flags().StringVar(&validatePath, "validate", "", "validate PATH as effective runtime config and exit")
	return cmd
}
