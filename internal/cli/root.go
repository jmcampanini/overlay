// Package cli defines overlay's CLI commands and output contracts.
package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/spf13/cobra"
)

// Version is injected at build time by the mise build task or package manager.
var Version = "n/a"

type commandOptions struct {
	color colorMode
}

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
	options := commandOptions{color: colorMode(config.DefaultColor)}
	var showVersion bool

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

Without a command, print this help on stdout. --version prints the build
version on stdout. Neither reads configuration or changes files.

` + contractHelp + `

Common tasks:
  Set up a machine:     overlay bootstrap --source-url URL --source-path PATH
  Preview a render:     overlay render --dry-run
  Render into home:     overlay render
  Diff home vs render:  overlay diff
  Show configuration:   overlay config
  Show values:          overlay values

Command groups:
  completion  Shell completion scripts:  overlay completion --help`,
		DisableSuggestions: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return usageError{err: err}
			}
			if showVersion {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "overlay version %s\n", Version)
				return err
			}
			return cmd.Help()
		},
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
	cmd.Flags().BoolVarP(&showVersion, "version", "v", false, "print the build version")
	cmd.PersistentFlags().Var(colorValue{mode: &options.color}, "color", "color output: auto, always, or never")
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddGroup(
		&cobra.Group{ID: "commands", Title: "Commands:"},
		&cobra.Group{ID: helpTopicsGroupID, Title: "Help topics:"},
	)
	help := helpCommand()
	cmd.SetHelpCommand(help)

	cmd.AddCommand(
		bootstrapCommand(),
		catCommand(),
		completionCommand(),
		configCommand(),
		diffCommand(),
		exitCodesTopic(),
		filesCommand(),
		help,
		renderCommand(&options),
		valuesCommand(),
	)
	for _, child := range cmd.Commands() {
		if child.GroupID == "" {
			child.GroupID = "commands"
		}
	}
	markUsageErrors(cmd)

	return cmd
}
