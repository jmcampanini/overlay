package discover

import (
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

// NewGlobIgnorer returns an Ignorer backed by doublestar glob patterns.
func NewGlobIgnorer(patterns []string) Ignorer {
	cleaned := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if p = strings.TrimSpace(p); p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return globIgnorer{patterns: cleaned}
}

func (g globIgnorer) Match(relPath string, isDir bool) bool {
	rel := filepath.ToSlash(relPath)
	base := filepath.Base(rel)
	for _, pattern := range g.patterns {
		dirOnly := strings.HasSuffix(pattern, "/")
		if dirOnly {
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
// A missing .gitignore file yields an ignorer that never matches.
func NewGitignoreIgnorer(root string) (Ignorer, error) {
	path := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(path); err != nil {
		return NoopIgnorer(), nil
	}
	ign, err := gitignore.CompileIgnoreFile(path)
	if err != nil {
		return nil, err
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
