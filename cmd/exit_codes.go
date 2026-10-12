package cmd

import "github.com/spf13/cobra"

func exitCodesTopic() *cobra.Command {
	return &cobra.Command{
		Use:   "exit-codes",
		Short: "Exit codes and error categories",
		Long: `overlay exits 0 or 1:

  0  Success. Outcomes that are not failures also exit 0: 'config' with no
     configuration file (it reports the defaults), 'bootstrap' finding a
     source already checked out, 'render' with nothing to write, --help,
     --version, and 'overlay help <topic>' naming an unknown topic, which
     prints 'Unknown help topic' and the root usage on stdout.
  1  Any failure. Usage errors (an unknown command, operand, or flag), a
     missing --config file, an invalid configuration, an existing file
     during bootstrap, a path that is not a checkout of its source url,
     git missing from PATH, and a failed git command print
     'overlay: error: <message>' on stderr. git's own output also goes to
     stderr. 'render' also fails, writing nothing, on a missing or invalid
     source manifest, a package entry that is not a regular file or a link
     to one, two files in one layer rendering to the same target, a
     template that fails to parse or execute, a missing target directory,
     or a directory at a target path; a failure
     while writing stops there and prints no plan, and files already
     written stay.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
