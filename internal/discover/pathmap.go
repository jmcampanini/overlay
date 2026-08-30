package discover

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// TargetPath resolves where a discovered overlay group should be written.
//
//	relDir:    path segment from source root to the group's directory
//	stem:      target filename stem, possibly containing dots
//	ext:       optional target filename extension without a leading dot
//	target:    configured target directory (may begin with ~ or $VAR)
//	dotPrefix: if true, rewrite dot- prefixed path segments to leading dots
func TargetPath(relDir, stem, ext, target string, dotPrefix bool) (string, error) {
	if target == "" {
		return "", fmt.Errorf("target directory is empty")
	}
	rel, err := TargetRelativePath(relDir, stem, ext, dotPrefix)
	if err != nil {
		return "", err
	}
	return targetPathFromRelative(target, rel)
}

func targetPathFromRelative(target, rel string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("target directory is empty")
	}
	expanded, err := ExpandPath(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(expanded, filepath.FromSlash(rel)), nil
}

// TargetRelativePath resolves a group's rendered path relative to the target.
func TargetRelativePath(relDir, stem, ext string, dotPrefix bool) (string, error) {
	rawSegments := strings.Split(filepath.ToSlash(relDir), "/")
	parts := make([]string, 0, len(rawSegments)+1)
	for _, seg := range rawSegments {
		if seg == "" {
			continue
		}
		if dotPrefix {
			seg = transformDotPrefix(seg)
		}
		parts = append(parts, seg)
	}
	if dotPrefix {
		stem = transformDotPrefix(stem)
	}
	name := stem
	if ext != "" {
		name += "." + ext
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid target filename %q", name)
	}
	parts = append(parts, name)
	return path.Join(parts...), nil
}

// ExpandPath expands a leading ~ to the user's home directory and any
// $VAR or ${VAR} sequences from the environment. Undefined environment
// variables produce an error rather than silently expanding to "" - so
// a typo in $XDG_CONIFG_HOME doesn't cause overlay to write to /foo.
func ExpandPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	var missing []string
	expanded := os.Expand(p, func(name string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		missing = append(missing, name)
		return ""
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("undefined environment variable(s): %s", strings.Join(missing, ", "))
	}
	return expanded, nil
}

// transformDotPrefix rewrites a single path segment: "dot-claude" -> ".claude".
// Segments that don't start with "dot-" are returned unchanged.
func transformDotPrefix(segment string) string {
	if after, ok := strings.CutPrefix(segment, "dot-"); ok {
		return "." + after
	}
	return segment
}
