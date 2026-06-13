package discover

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	gitignore "github.com/sabhiram/go-gitignore"
)

// Ignorer reports whether a given relative path should be skipped.
type Ignorer interface {
	Match(relPath string, isDir bool) bool
}

type noopIgnorer struct{}

func (noopIgnorer) Match(string, bool) bool { return false }

// NoopIgnorer returns an Ignorer that never matches.
func NoopIgnorer() Ignorer { return noopIgnorer{} }

type globIgnorer struct {
	patterns []string
}

// ValidateGlobPatterns reports malformed ignore glob patterns without
// constructing an Ignorer.
func ValidateGlobPatterns(patterns []string) error {
	_, err := NormalizeGlobPatterns(patterns)
	return err
}

// NormalizeGlobPatterns trims and drops empty glob patterns, and reports
// malformed doublestar globs. It is shared by the ignore and
// substitute_exclude fields, so the error names the pattern, not the field.
func NormalizeGlobPatterns(patterns []string) ([]string, error) {
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		check := strings.TrimSuffix(pattern, "/")
		if _, err := doublestar.Match(check, ""); err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		normalized = append(normalized, pattern)
	}
	return normalized, nil
}

// NewGlobIgnorer returns an Ignorer backed by doublestar glob patterns.
// Patterns are validated up front; a malformed pattern is returned as an
// error rather than silently producing no matches at run time.
func NewGlobIgnorer(patterns []string) (Ignorer, error) {
	normalized, err := NormalizeGlobPatterns(patterns)
	if err != nil {
		return nil, err
	}
	return globIgnorer{patterns: normalized}, nil
}

func (g globIgnorer) Match(relPath string, isDir bool) bool {
	rel := filepath.ToSlash(relPath)
	base := filepath.Base(rel)
	for _, pattern := range g.patterns {
		if strings.HasSuffix(pattern, "/") {
			if !isDir {
				continue
			}
			pattern = strings.TrimSuffix(pattern, "/")
		}
		if match, _ := doublestar.Match(pattern, rel); match {
			return true
		}
		if match, _ := doublestar.Match(pattern, base); match {
			return true
		}
	}
	return false
}

type gitignoreIgnorer struct {
	ign *gitignore.GitIgnore
}

// NewGitignoreIgnorer loads .gitignore rules from the given root directory.
// A missing .gitignore file yields an ignorer that never matches; any other
// stat error is propagated so the caller can surface it.
func NewGitignoreIgnorer(root string) (Ignorer, error) {
	path := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NoopIgnorer(), nil
		}
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	ign, err := gitignore.CompileIgnoreFile(path)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", path, err)
	}
	return gitignoreIgnorer{ign: ign}, nil
}

func (g gitignoreIgnorer) Match(relPath string, _ bool) bool {
	if g.ign == nil {
		return false
	}
	return g.ign.MatchesPath(filepath.ToSlash(relPath))
}

type chain struct {
	ignorers []Ignorer
}

// NewChain composes multiple ignorers with OR semantics.
func NewChain(ignorers ...Ignorer) Ignorer {
	out := make([]Ignorer, 0, len(ignorers))
	for _, i := range ignorers {
		if i == nil {
			continue
		}
		out = append(out, i)
	}
	return chain{ignorers: out}
}

func (c chain) Match(relPath string, isDir bool) bool {
	for _, i := range c.ignorers {
		if i.Match(relPath, isDir) {
			return true
		}
	}
	return false
}
