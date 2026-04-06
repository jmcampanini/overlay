// Package logging wires the charmbracelet/log logger used throughout overlay.
package logging

import (
	"os"

	"github.com/charmbracelet/log"
)

// Setup returns a logger writing to stderr with a level chosen from the
// quiet/verbose flags. Verbose beats quiet if both are set.
func Setup(quiet, verbose bool) *log.Logger {
	l := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})
	switch {
	case verbose:
		l.SetLevel(log.DebugLevel)
	case quiet:
		l.SetLevel(log.WarnLevel)
	default:
		l.SetLevel(log.InfoLevel)
	}
	return l
}
