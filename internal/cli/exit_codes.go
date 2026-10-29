package cli

import "github.com/spf13/cobra"

func exitCodesTopic() *cobra.Command {
	return &cobra.Command{
		Use:   "exit-codes",
		Short: "Exit codes and error categories",
		Long: `overlay exits 0, 1, or 2. A failure prints one line on stderr,
'overlay: <message>', and no usage text. git's own output during bootstrap
also goes to stderr.

  0  Success. Outcomes that are not failures also exit 0: 'config' with no
     configuration file (it reports the defaults), 'bootstrap' finding a
     source already checked out, 'render' with nothing to write, 'diff'
     whether or not anything differs, 'files' and 'cat' with no matches,
     --help, --version, and a bare
     command group, which prints its help.
  1  Any failure other than a usage error: a missing --config file, an
     invalid configuration, an existing file during bootstrap, a path that
     is not a checkout of its source url, git missing from PATH, and a
     failed git command. 'render' also fails, writing nothing, on a
     missing or invalid source manifest, a package entry that is not a
     regular file or a link to one, two files in one layer rendering to
     the same target, a template that fails to parse or execute, a
     malformed fragment name, a fragment that fails to parse, substitute,
     or merge, a fragment in a later layer than a plain file at its
     target, a missing target directory, or a directory at a target path;
     a failure while writing stops there and prints no plan, and files
     already written stay. 'diff' fails on the same conditions and on a
     target path holding anything other than a regular file or a symbolic
     link; the patch printed before the failure stays on stdout. 'files'
     and 'cat' fail before output on a missing or invalid source manifest,
     a stat error other than absence, or one pattern matching in two
     sources in the same layer. 'cat' also fails before output on an
     unreadable file, and 'files' on a path containing a newline.
  2  A usage error: an unknown command, help topic, operand, or flag, or a
     flag value that does not parse, such as '--color sometimes'. overlay
     rejects the invocation before any command work. 'files' and 'cat'
     require at least one pattern. A pattern must be nonempty, contain
     {layer}, not start with / or ~, have no .. path segment, have no
     braces outside {layer}, and contain none of *, ?, or [.

'overlay help exit-codes' prints this text on stdout, prompts for nothing,
and needs no terminal.`,
		GroupID: helpTopicsGroupID,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
