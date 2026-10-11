package cmd

import "github.com/spf13/cobra"

func exitCodesTopic() *cobra.Command {
	return &cobra.Command{
		Use:   "exit-codes",
		Short: "Exit codes and error categories",
		Long: `overlay exits 0 or 1:

  0  Success. Outcomes that are not failures also exit 0: 'config' with no
     configuration file (it reports the defaults), 'bootstrap' finding a
     source already checked out, --help, --version, and 'overlay help
     <topic>' naming an unknown topic, which prints 'Unknown help topic'
     and the root usage on stdout.
  1  Any failure. Usage errors (an unknown command, operand, or flag), a
     missing --config file, an invalid configuration, an existing file
     during bootstrap, a path that is not a checkout of its source url,
     git missing from PATH, and a failed git command print
     'overlay: error: <message>' on stderr. git's own output also goes to
     stderr.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
