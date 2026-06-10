// Package document parses and serializes overlay source documents.
// It isolates format-specific concerns (JSON, TOML, YAML) from the merge logic.
package document

import (
	"fmt"
	"strings"
)

// Format identifies a supported document format.
type Format int

// Format constants identify the supported document formats.
const (
	FormatUnknown Format = iota // unrecognized
	FormatJSON                  // RFC 8259 JSON
	FormatTOML                  // TOML 1.0
	FormatYAML                  // YAML 1.2 config-style documents
	FormatCopy                  // whole-file copy-through
)

func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatTOML:
		return "toml"
	case FormatYAML:
		return "yaml"
	case FormatCopy:
		return "copy"
	}
	return "unknown"
}

// DetectFormat returns the format for a filename based on its extension.
func DetectFormat(filename string) (Format, error) {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".json"):
		return FormatJSON, nil
	case strings.HasSuffix(lower, ".toml"):
		return FormatTOML, nil
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return FormatYAML, nil
	}
	return FormatUnknown, fmt.Errorf("unsupported document format for %q", filename)
}

// Parse decodes the bytes for the given format into a generic value tree
// (map[string]any / []any / scalar leaves) ready for merging.
func Parse(data []byte, f Format) (any, error) {
	switch f {
	case FormatJSON:
		return parseJSON(data)
	case FormatTOML:
		return parseTOML(data)
	case FormatYAML:
		return parseYAML(data)
	}
	return nil, fmt.Errorf("unsupported format: %s", f)
}

// SerializeOptions controls format-specific output style.
type SerializeOptions struct {
	TOMLIndentTables bool
}

// Serialize encodes the value tree for the given format. Output is
// deterministic (keys are alphabetized) for JSON, TOML, and YAML.
func Serialize(v any, f Format) ([]byte, error) {
	return SerializeWithOptions(v, f, SerializeOptions{})
}

// SerializeWithOptions encodes the value tree for the given format using opts.
func SerializeWithOptions(v any, f Format, opts SerializeOptions) ([]byte, error) {
	switch f {
	case FormatJSON:
		return serializeJSON(v)
	case FormatTOML:
		return serializeTOML(v, opts.TOMLIndentTables)
	case FormatYAML:
		return serializeYAML(v)
	}
	return nil, fmt.Errorf("unsupported format: %s", f)
}
