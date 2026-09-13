package cmd

import "github.com/spf13/cobra"

func exitCodesTopic() *cobra.Command {
	return &cobra.Command{
		Use:   "exit-codes",
		Short: "Exit codes and error categories",
		Long: `overlay exits 0, 1, or 2. 'diff' and 'orphans' report their result in the
exit status, so they use 2 for failures; every other command uses 1.

  0  Success. 'render' wrote every target, 'plan' printed its table,
     'diff' found no drift, 'orphans' found no orphan, 'config' printed
     its report (even when its '# effective_errors:' block lists
     problems), 'config --validate' found the file valid, and 'docs'
     printed the reference. --help, --version, a bare 'overlay', and this
     topic also exit 0. A configured source directory that does not exist
     and a group with no active layer are skipped with a log line, not a
     failure. 'overlay help NAME' with an unknown NAME prints 'Unknown
     help topic' and the root usage on stderr and still exits 0.
  1  'diff': at least one target differs from its rendered output.
     'orphans': at least one orphan was found. Both print the result on
     stdout and nothing about it on stderr; --json does not change the
     status. Every other command: any failure, printed as 'overlay:
     <message>' on stderr. That covers usage errors (an unknown command
     or flag), a --config file that cannot be loaded, an invalid
     .overlay.toml (an unknown key, a bad value, an invalid render_rules,
     ignore, or substitute_exclude entry, a reserved profile name, a
     missing target), a pin matching no substitute selector, for
     'plan' a discovery error or a substituting target with missing
     variables or that cannot compose, and for 'render' a discovery
     error, a layer that cannot be parsed, a missing variable, a target
     that cannot be written, or a state file that cannot be read or
     saved. With --continue, 'render' writes the clean targets, logs each
     failure, and still exits 1.
     'config --validate' lists every effective error and exits 1.
  2  'diff' and 'orphans': any failure. Resolution errors print 'overlay:
     <message>'; discovery, render, read, state, path, and I/O errors
     print as ERRO log lines. 'orphans' leaves stdout empty on exit 2.
     'diff' stops before printing any diff when a group fails to compose,
     and where it is when a target cannot be read; with --continue it
     logs each failure, prints every clean diff, and still exits 2.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
