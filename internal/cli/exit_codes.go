package cli

import "github.com/spf13/cobra"

func exitCodesTopic() *cobra.Command {
	return newHelpTopic("exit-codes", "Exit codes and error categories",
		`overlay exits 0, 1, or 2. A failure prints one line on stderr,
'overlay: <message>', and no usage text. git's own output during bootstrap
also goes to stderr.

  0  Success. Outcomes that are not failures also exit 0: 'config' with no
     configuration file (it reports the defaults), 'bootstrap' finding a
     source already checked out, 'render' with nothing to write, 'diff'
     whether or not anything differs, --help, --version, and 'overlay
     help <topic>' naming an unknown topic, which prints 'Unknown help
     topic' and the root usage on stdout.
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
     link; the patch printed before the failure stays on stdout.
  2  A usage error: an unknown command, operand, or flag, or a flag value
     that does not parse, such as '--color sometimes'. overlay rejects the
     invocation before any command work.

'overlay help exit-codes' prints this text on stdout, prompts for nothing,
and needs no terminal.`)
}

// helpTopicAnnotation marks a command built by newHelpTopic so the help and
// grammar tests can tell topics from commands that do work.
const helpTopicAnnotation = "overlay.help-topic"

// newHelpTopic builds a help topic: a runnable command that does no work and
// prints its help whether invoked directly or through 'overlay help NAME'.
func newHelpTopic(name, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:         name,
		Short:       short,
		Long:        long,
		Annotations: map[string]string{helpTopicAnnotation: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
