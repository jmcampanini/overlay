package cmd

import "fmt"

// ExitCode lets main return a command-defined status without duplicate output.
type ExitCode int

// Error satisfies the error interface.
func (e ExitCode) Error() string { return fmt.Sprintf("exit code %d", int(e)) }
