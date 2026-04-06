package merge

import (
	"reflect"
	"testing"
	"time"
)

func TestMergeMapsDeep(t *testing.T) {
	base := map[string]any{
		"a": map[string]any{"b": 1, "c": 2},
		"d": 3,
	}
	override := map[string]any{
		"a": map[string]any{"b": 10, "e": 5},
	}
	got := Merge(base, override)
	want := map[string]any{
		"a": map[string]any{"b": 10, "c": 2, "e": 5},
		"d": 3,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// base must not be mutated
	if base["a"].(map[string]any)["b"] != 1 {
		t.Error("base map was mutated")
	}
}

func TestMergeScalarListDedupe(t *testing.T) {
	base := map[string]any{"allow": []any{"a", "b"}}
	override := map[string]any{"allow": []any{"b", "c"}}
	got := Merge(base, override)
	want := map[string]any{"allow": []any{"a", "b", "c"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeObjectListConcat(t *testing.T) {
	base := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "foo.sh"},
		},
	}
	override := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "bar.sh"},
		},
	}
	got := Merge(base, override)
	want := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "foo.sh"},
			map[string]any{"type": "command", "command": "bar.sh"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeScalarReplace(t *testing.T) {
	base := map[string]any{"a": "old", "b": 1}
	override := map[string]any{"a": "new", "b": 2}
	got := Merge(base, override)
	want := map[string]any{"a": "new", "b": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeTypeMismatchOverrideWins(t *testing.T) {
	base := map[string]any{"a": "string"}
	override := map[string]any{"a": 42}
	got := Merge(base, override)
	want := map[string]any{"a": 42}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeMapOverridesScalar(t *testing.T) {
	base := map[string]any{"a": "string"}
	override := map[string]any{"a": map[string]any{"nested": true}}
	got := Merge(base, override)
	want := map[string]any{"a": map[string]any{"nested": true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeListOfMixedScalarsIsScalarList(t *testing.T) {
	base := []any{"a", 1, true}
	override := []any{"a", 2.5}
	got := Merge(base, override)
	want := []any{"a", 1, true, 2.5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeEmptyListsAreScalarLists(t *testing.T) {
	base := []any{}
	override := []any{"a"}
	got := Merge(base, override)
	want := []any{"a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeListWithNilIsObjectList(t *testing.T) {
	// Matches Python: [None] fails isinstance check, so it's treated as
	// a non-scalar list -> concat without dedupe.
	base := []any{"a", nil, "b"}
	override := []any{"a", nil}
	got := Merge(base, override)
	want := []any{"a", nil, "b", "a", nil}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeListWithTimeIsObjectList(t *testing.T) {
	t1 := time.Now()
	base := []any{t1}
	override := []any{t1}
	got := Merge(base, override)
	want := []any{t1, t1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeIntAndFloatScalarsTogether(t *testing.T) {
	// JSON decodes numbers as float64; TOML decodes ints as int64.
	// Both kinds should be treated as scalars.
	base := []any{int64(1), int64(2)}
	override := []any{float64(2), float64(3)}
	got := Merge(base, override)
	// int64(2) != float64(2) in Go map key semantics, so both pass through dedupe.
	want := []any{int64(1), int64(2), float64(2), float64(3)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeDeeplyNested(t *testing.T) {
	base := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": []any{"x"},
				},
			},
		},
	}
	override := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": []any{"y"},
				},
			},
		},
	}
	got := Merge(base, override)
	want := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": []any{"x", "y"},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeNilBaseReturnsOverride(t *testing.T) {
	got := Merge(nil, map[string]any{"a": 1})
	want := map[string]any{"a": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeEmptyMapBaseAndMapOverride(t *testing.T) {
	got := Merge(map[string]any{}, map[string]any{"a": 1})
	want := map[string]any{"a": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeObjectListBecauseOfNestedMap(t *testing.T) {
	base := []any{map[string]any{"k": "v"}}
	override := []any{"scalar"}
	got := Merge(base, override)
	want := []any{map[string]any{"k": "v"}, "scalar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeDedupeCollapsesBaseInternalDuplicates(t *testing.T) {
	// Pin the contract: even if base contains duplicates, dedupe collapses
	// them. Override is empty so the only thing exercised is the seen-set
	// handling on the base side.
	base := []any{"a", "b", "a", "c"}
	override := []any{"d"}
	got := Merge(base, override)
	want := []any{"a", "b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeDedupePreservesOrder(t *testing.T) {
	base := []any{"a", "b", "c"}
	override := []any{"b", "d", "a", "e"}
	got := Merge(base, override)
	want := []any{"a", "b", "c", "d", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeDoesNotMutateBase(t *testing.T) {
	base := map[string]any{
		"list": []any{"a", "b"},
		"map":  map[string]any{"k": "v"},
	}
	override := map[string]any{
		"list": []any{"c"},
		"map":  map[string]any{"k": "new"},
	}
	_ = Merge(base, override)
	if !reflect.DeepEqual(base["list"], []any{"a", "b"}) {
		t.Errorf("base list was mutated: %v", base["list"])
	}
	if !reflect.DeepEqual(base["map"], map[string]any{"k": "v"}) {
		t.Errorf("base map was mutated: %v", base["map"])
	}
}
