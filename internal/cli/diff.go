package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/diff"
)

// DiffExitCode wraps an exit code so main can detect it and exit without
// the generic "error" wrapping that would print an extra message.
type DiffExitCode int

// Error satisfies the error interface.
func (e DiffExitCode) Error() string { return fmt.Sprintf("diff exit code %d", int(e)) }

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [source...]",
		Short: "Show a unified diff between rendered output and existing target files.",
		Long:  "Render each overlay group in memory and print a unified diff against the\nexisting target files. Positional sources select package roots for this run.\n" + sourceSelectionHelp + "\n" + diffOutputHelp + "\n" + profilePrecedenceHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := Resolve(cmd, &globals, args...)
			if err != nil {
				fmt.Fprintln(os.Stderr, "overlay:", err)
				return DiffExitCode(2)
			}
			differ, err := diff.Run(diff.Options{
				Settings:         r.Settings,
				ContinueOnError:  r.ContinueOnError,
				TOMLIndentTables: r.RawConfig.TOMLIndentTables,
				Logger:           r.Logger,
				Out:              os.Stdout,
			})
			if err != nil {
				r.Logger.Error(err)
				return DiffExitCode(2)
			}
			if differ {
				return DiffExitCode(1)
			}
			return nil
		},
		SilenceErrors: true,
	}
	return cmd
}
