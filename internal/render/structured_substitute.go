package render

import (
	"fmt"
	"maps"
	"slices"

	"github.com/jmcampanini/overlay/internal/substitute"
)

type substitutionCollector struct {
	consumed map[string]struct{}
	missing  map[string]struct{}
}

func (c *substitutionCollector) add(result substitute.Result) {
	if len(result.Consumed) > 0 && c.consumed == nil {
		c.consumed = make(map[string]struct{}, len(result.Consumed))
	}
	for _, name := range result.Consumed {
		c.consumed[name] = struct{}{}
	}
	if len(result.Missing) > 0 && c.missing == nil {
		c.missing = make(map[string]struct{}, len(result.Missing))
	}
	for _, name := range result.Missing {
		c.missing[name] = struct{}{}
	}
}

func (c *substitutionCollector) result() substitute.Result {
	return substitute.Result{
		Consumed: sortedStringSet(c.consumed),
		Missing:  sortedStringSet(c.missing),
	}
}

func sortedStringSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}

func substituteTree(v any, r *substitute.Resolver, c *substitutionCollector, path string) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		return substituteMap(x, r, c, path)
	case []any:
		out := make([]any, len(x))
		var firstErr error
		for i, item := range x {
			value, err := substituteTree(item, r, c, fmt.Sprintf("%s[%d]", path, i))
			if err != nil && firstErr == nil {
				firstErr = err
			}
			out[i] = value
		}
		if firstErr != nil {
			return nil, firstErr
		}
		return out, nil
	case string:
		out, result := r.Apply([]byte(x))
		c.add(result)
		return string(out), nil
	default:
		return v, nil
	}
}

func substituteMap(in map[string]any, r *substitute.Resolver, c *substitutionCollector, path string) (map[string]any, error) {
	out := make(map[string]any, len(in))
	var firstErr error
	for _, key := range slices.Sorted(maps.Keys(in)) {
		keyOut, keyResult := r.Apply([]byte(key))
		c.add(keyResult)
		newKey := string(keyOut)
		value, err := substituteTree(in[key], r, c, substitutionPath(path, newKey))
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if len(keyResult.Missing) > 0 {
			continue
		}
		if _, exists := out[newKey]; exists {
			if firstErr == nil {
				firstErr = fmt.Errorf("key collision at %s: %q", path, newKey)
			}
			continue
		}
		out[newKey] = value
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func substitutionPath(parent, key string) string {
	quoted := fmt.Sprintf("%q", key)
	if parent == "$" {
		return "$[" + quoted + "]"
	}
	return parent + "[" + quoted + "]"
}
