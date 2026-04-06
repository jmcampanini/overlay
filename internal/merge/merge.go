// Package merge implements the overlay merge algorithm.
//
// Rules:
//   - map + map: deep-merge recursively; override keys overwrite base keys
//   - scalar list + scalar list: dedupe-and-concat preserving first-seen order
//   - object list + object list: concat, no dedupe
//   - anything else: override replaces base
package merge

import (
	"maps"
	"reflect"
	"time"
)

// Merge returns the result of overlaying override on top of base.
// Neither argument is mutated; the result is a fresh value tree.
func Merge(base, override any) any {
	baseMap, baseIsMap := base.(map[string]any)
	overrideMap, overrideIsMap := override.(map[string]any)
	if baseIsMap && overrideIsMap {
		return mergeMaps(baseMap, overrideMap)
	}

	baseList, baseIsList := base.([]any)
	overrideList, overrideIsList := override.([]any)
	if baseIsList && overrideIsList {
		if isScalarList(baseList) && isScalarList(overrideList) {
			return dedupeConcat(baseList, overrideList)
		}
		return concat(baseList, overrideList)
	}

	return override
}

func mergeMaps(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(override))
	maps.Copy(result, base)
	for k, v := range override {
		if existing, ok := result[k]; ok {
			result[k] = Merge(existing, v)
		} else {
			result[k] = v
		}
	}
	return result
}

// isScalarList reports whether every element is a scalar (string, bool,
// or any int/float kind). Empty lists count as scalar lists. nil elements
// are NOT scalars.
func isScalarList(items []any) bool {
	for _, item := range items {
		if !isScalar(item) {
			return false
		}
	}
	return true
}

func isScalar(v any) bool {
	if v == nil {
		return false
	}
	if _, ok := v.(time.Time); ok {
		return false
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func dedupeConcat(base, override []any) []any {
	result := make([]any, 0, len(base)+len(override))
	seen := make(map[any]struct{}, len(base)+len(override))
	for _, item := range base {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	for _, item := range override {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func concat(base, override []any) []any {
	result := make([]any, 0, len(base)+len(override))
	result = append(result, base...)
	result = append(result, override...)
	return result
}
