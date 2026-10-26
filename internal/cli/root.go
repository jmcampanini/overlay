// Package cli defines overlay's CLI commands and output contracts.
package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// Version is injected at build time by the mise build task or package manager.
var Version = "n/a"

// Exit statuses; the exit-codes help topic documents them.
const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

// Execute runs the command tree and returns the process exit status. A
// failure is one stderr line, so line breaks inside the message become
// spaces.
func Execute(args []string, out, errOut io.Writer) int {
	root := newRootCommand(out, errOut)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		return exitSuccess
	}
	_, _ = fmt.Fprintf(errOut, "overlay: %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
	if isUsageError(root, args, err) {
		return exitUsage
	}
	return exitFailure
}

// usageError marks an invocation the command library rejected before any
// command work: an unknown flag or flag value, or operands that fail a
// command's positional grammar.
type usageError struct {
	err error
}

func (e usageError) Error() string { return e.err.Error() }

func (e usageError) Unwrap() error { return e.err }

// isUsageError reports whether err is a usage error. Cobra returns them
// untyped: the root's flag error func wraps flag errors and newRootCommand
// wraps every Args validator, which leaves an unknown command, found by
// repeating the command lookup, which has no side effects.
func isUsageError(root *cobra.Command, args []string, err error) bool {
	var usage usageError
	if errors.As(err, &usage) {
		return true
	}
	_, _, findErr := root.Find(args)
	return findErr != nil
}

// markUsageErrors wraps the Args validator of cmd and its descendants so a
// positional grammar failure carries usageError.
func markUsageErrors(cmd *cobra.Command) {
	if validate := cmd.Args; validate != nil {
		cmd.Args = func(cmd *cobra.Command, args []string) error {
			if err := validate(cmd, args); err != nil {
				return usageError{err: err}
			}
			return nil
		}
	}
	for _, child := range cmd.Commands() {
		markUsageErrors(child)
	}
}

// newRootCommand builds the overlay command tree.
func newRootCommand(out, errOut io.Writer) *cobra.Command {
	// The long description ends with the task list: the command group block
	// that would follow it is empty because overlay has no first-level
	// command groups.
	cmd := &cobra.Command{
		Use:   "overlay",
		Short: "Render layered dotfiles from source repositories",
		Long: `Render layered dotfiles from git source repositories into a target
directory.

overlay reads one configuration file that lists the layers to apply and the
git repositories that provide files, and writes the files those layers
select into the target directory.

` + configFileHelp + `

Only 'overlay bootstrap' runs an external program: it runs git from PATH to
clone sources and to read the origin of an existing checkout, and never
lets git prompt for credentials. No command reads the network on its own.

` + contractHelp + `

Common tasks:
  Set up a machine:     overlay bootstrap --source-url URL --source-path PATH
  Preview a render:     overlay render --dry-run
  Render into home:     overlay render
  Diff home vs render:  overlay diff
  Show configuration:   overlay config`,
		Version:            Version,
		DisableSuggestions: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
	}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err: err}
	})
	// Register --help and --version before Find strips flags from the
	// arguments. Cobra otherwise adds them during execute, after Find has
	// already treated an unregistered --help or --version as taking the next
	// argument as its value.
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()

	cmd.AddCommand(
		bootstrapCommand(),
		configCommand(),
		diffCommand(),
		exitCodesTopic(),
		renderCommand(),
	)
	markUsageErrors(cmd)

	return cmd
}
