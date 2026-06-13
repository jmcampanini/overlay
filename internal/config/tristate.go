package config

import "fmt"

// TriState is a three-valued boolean for TOML fields that distinguish
// "explicitly true", "explicitly false", and "not set" (inherit). A plain
// *bool cannot be used because the config loader rejects pointer fields.
type TriState string

const (
	// TriStateUnset means the field was not present and inherits a default.
	TriStateUnset TriState = ""
	// TriStateTrue is an explicit TOML `true`.
	TriStateTrue TriState = "true"
	// TriStateFalse is an explicit TOML `false`.
	TriStateFalse TriState = "false"
)

// Bool returns the explicit value and whether one was set.
func (t TriState) Bool() (value, set bool) {
	switch t {
	case TriStateTrue:
		return true, true
	case TriStateFalse:
		return false, true
	default:
		return false, false
	}
}

// UnmarshalTOML accepts only TOML booleans, so `substitute = "true"` is
// rejected rather than silently coerced.
func (t *TriState) UnmarshalTOML(v any) error {
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("expected a boolean, got %T", v)
	}
	if b {
		*t = TriStateTrue
	} else {
		*t = TriStateFalse
	}
	return nil
}

// MarshalTOML emits a bare boolean literal so config echoes round-trip as
// `substitute = true`, not a quoted string.
func (t TriState) MarshalTOML() ([]byte, error) {
	switch t {
	case TriStateTrue, TriStateFalse:
		return []byte(t), nil
	default:
		return nil, fmt.Errorf("cannot marshal unset TriState; use omitempty")
	}
}
