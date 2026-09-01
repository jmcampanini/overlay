// Package substitute implements selector-gated ${NAME} variable substitution
// over composed render output, with $$-doubling as the literal escape.
package substitute

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ValidName reports whether s matches the POSIX environment-variable name
// charset: [A-Za-z_][A-Za-z0-9_]*.
func ValidName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ValidSelector reports whether s is a valid exact variable name or a valid
// variable name prefix followed by "*". A bare "*" is invalid.
func ValidSelector(s string) bool {
	prefix, isPrefix := strings.CutSuffix(s, "*")
	if isPrefix {
		return ValidName(prefix)
	}
	return ValidName(s)
}

// ParsePins parses repeated NAME=value entries into a pin map. Later entries
// overwrite earlier ones. NAME= pins the empty string; a missing '=' or an
// invalid name is an error.
func ParsePins(entries []string) (map[string]string, error) {
	pins := make(map[string]string, len(entries))
	for _, entry := range entries {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			return nil, fmt.Errorf("pin %q must have the form NAME=value", entry)
		}
		if !ValidName(name) {
			return nil, fmt.Errorf("pin name %q is not a valid variable name ([A-Za-z_][A-Za-z0-9_]*)", name)
		}
		pins[name] = value
	}
	return pins, nil
}

// DeadPins returns the sorted pin names that match no configured selector and
// therefore can never be referenced by any target.
func DeadPins(pins map[string]string, selectors []string) []string {
	var dead []string
	for name := range pins {
		if !matchesSelector(name, selectors) {
			dead = append(dead, name)
		}
	}
	slices.Sort(dead)
	return dead
}

// Result reports the variable names referenced by one Apply call. Missing is
// the subset of Consumed that had no value (neither pinned nor in the
// environment snapshot).
type Result struct {
	Consumed []string
	Missing  []string
}

// Resolver substitutes selector-gated ${NAME} references using a single
// environment snapshot overlaid with pins. It accumulates consumed names
// across Apply calls so unused pins can be reported once per invocation.
// Not safe for concurrent use.
type Resolver struct {
	selectors []string
	values    map[string]string
	pins      map[string]string
	consumed  map[string]struct{}
}

// NewResolver snapshots environ (in "K=V" form, as returned by os.Environ)
// and overlays pins on top, so pinned values win over the ambient
// environment.
func NewResolver(selectors []string, pins map[string]string, environ []string) *Resolver {
	values := make(map[string]string, len(environ)+len(pins))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			values[k] = v
		}
	}
	for k, v := range pins {
		values[k] = v
	}
	return &Resolver{
		selectors: slices.Clone(selectors),
		values:    values,
		pins:      maps.Clone(pins),
		consumed:  make(map[string]struct{}),
	}
}

// Enabled reports whether substitution is on: a non-empty selector list is the
// feature's global switch.
func (r *Resolver) Enabled() bool {
	return r != nil && len(r.selectors) > 0
}

// Apply substitutes references in one target's composed content with a single
// left-to-right pass. Substituted values are never re-scanned. The escape
// $${NAME} emits a literal ${NAME} and is recognized exactly where the
// reference itself would otherwise match; all other text passes through
// byte-identical.
func (r *Resolver) Apply(content []byte) ([]byte, Result) {
	if !r.Enabled() || bytes.IndexByte(content, '$') < 0 {
		return content, Result{}
	}
	out := make([]byte, 0, len(content))
	consumed := map[string]struct{}{}
	missing := map[string]struct{}{}
	i := 0
	for i < len(content) {
		c := content[i]
		if c != '$' {
			out = append(out, c)
			i++
			continue
		}
		if i+1 < len(content) && content[i+1] == '$' {
			if name, end, ok := matchReference(content, i+2); ok && matchesSelector(name, r.selectors) {
				out = append(out, content[i+1:end]...)
				i = end
				continue
			}
			out = append(out, '$')
			i++
			continue
		}
		if name, end, ok := matchReference(content, i+1); ok && matchesSelector(name, r.selectors) {
			consumed[name] = struct{}{}
			r.consumed[name] = struct{}{}
			if value, found := r.values[name]; found {
				out = append(out, value...)
			} else {
				missing[name] = struct{}{}
			}
			i = end
			continue
		}
		// A ${...} that is not a matching reference passes through byte-identical,
		// together with everything up to the brace that balances the outer "{" -
		// or to end of input if it never closes. Tracking depth means a nested
		// reference inside an unsupported expression, whether balanced
		// (${X:-${PRE_Y}}) or unbalanced (${BROKEN${PRE_Y}), is never substituted.
		if i+1 < len(content) && content[i+1] == '{' {
			end, depth := i+2, 1
			for end < len(content) && depth > 0 {
				switch content[end] {
				case '{':
					depth++
				case '}':
					depth--
				}
				end++
			}
			out = append(out, content[i:end]...)
			i = end
			continue
		}
		out = append(out, '$')
		i++
	}
	return out, Result{Consumed: sortedKeys(consumed), Missing: sortedKeys(missing)}
}

// UnusedPins returns the sorted pin names not consumed by any Apply call so
// far. A nil resolver has no pins.
func (r *Resolver) UnusedPins() []string {
	if r == nil {
		return nil
	}
	var unused []string
	for name := range r.pins {
		if _, ok := r.consumed[name]; !ok {
			unused = append(unused, name)
		}
	}
	slices.Sort(unused)
	return unused
}

// matchReference matches `{NAME}` starting at content[j] (the byte after the
// dollar sign(s)) and returns the name and the index just past the closing
// brace.
func matchReference(content []byte, j int) (string, int, bool) {
	if j >= len(content) || content[j] != '{' {
		return "", 0, false
	}
	k := j + 1
	for k < len(content) && content[k] != '}' {
		k++
	}
	if k >= len(content) {
		return "", 0, false
	}
	name := string(content[j+1 : k])
	if !ValidName(name) {
		return "", 0, false
	}
	return name, k + 1, true
}

func matchesSelector(name string, selectors []string) bool {
	for _, selector := range selectors {
		if prefix, isPrefix := strings.CutSuffix(selector, "*"); isPrefix {
			if strings.HasPrefix(name, prefix) {
				return true
			}
			continue
		}
		if name == selector {
			return true
		}
	}
	return false
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}
