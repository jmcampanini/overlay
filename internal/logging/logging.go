// Package logging wires the Charm log logger used throughout overlay.
package logging

import (
	"os"

	"charm.land/log/v2"
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

// WarnMissingSources emits one warning for each skipped source directory.
func WarnMissingSources(l *log.Logger, sources []string) {
	if l == nil {
		l = log.Default()
	}
	for _, source := range sources {
		l.Warnf("source %q not found, skipping", source)
	}
}
