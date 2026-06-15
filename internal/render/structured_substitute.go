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
	c.consumed = addStringSet(c.consumed, result.Consumed)
	c.missing = addStringSet(c.missing, result.Missing)
}

func addStringSet(set map[string]struct{}, names []string) map[string]struct{} {
	if len(names) == 0 {
		return set
	}
	if set == nil {
		set = make(map[string]struct{}, len(names))
	}
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
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

func substituteTree(value any, resolver *substitute.Resolver, collector *substitutionCollector, path string) (any, error) {
	switch node := value.(type) {
	case map[string]any:
		return substituteMap(node, resolver, collector, path)
	case []any:
		output := make([]any, len(node))
		var firstErr error
		for i, item := range node {
			substituted, err := substituteTree(item, resolver, collector, fmt.Sprintf("%s[%d]", path, i))
			if err != nil && firstErr == nil {
				firstErr = err
			}
			output[i] = substituted
		}
		if firstErr != nil {
			return nil, firstErr
		}
		return output, nil
	case string:
		substituted, result := resolver.Apply([]byte(node))
		collector.add(result)
		return string(substituted), nil
	default:
		return value, nil
	}
}

func substituteMap(input map[string]any, resolver *substitute.Resolver, collector *substitutionCollector, path string) (map[string]any, error) {
	output := make(map[string]any, len(input))
	var firstErr error
	for _, key := range slices.Sorted(maps.Keys(input)) {
		substitutedKeyBytes, keyResult := resolver.Apply([]byte(key))
		collector.add(keyResult)
		substitutedKey := string(substitutedKeyBytes)
		substitutedValue, err := substituteTree(input[key], resolver, collector, substitutionPath(path, substitutedKey))
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if len(keyResult.Missing) > 0 {
			continue
		}
		if _, exists := output[substitutedKey]; exists {
			if firstErr == nil {
				firstErr = fmt.Errorf("key collision at %s: %q", path, substitutedKey)
			}
			continue
		}
		output[substitutedKey] = substitutedValue
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return output, nil
}

func substitutionPath(parent, key string) string {
	quotedKey := fmt.Sprintf("%q", key)
	if parent == "$" {
		return "$[" + quotedKey + "]"
	}
	return parent + "[" + quotedKey + "]"
}
