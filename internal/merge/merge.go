// Package merge composes the layered fragments of one JSON or TOML file into
// a single document. Each fragment is parsed on its own, every string value
// and map key gets ${NAME} environment substitution, the fragments deep-merge
// in the order given, and the result serializes with sorted keys.
package merge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Format is the document language of a set of fragments.
type Format string

const (
	// JSON fragments parse as JSON objects and serialize with two-space
	// indentation.
	JSON Format = "json"
	// TOML fragments parse as TOML tables and serialize flush-left.
	TOML Format = "toml"
)

// FormatOf returns the Format named by a file extension without its dot, and
// false when the extension is not one merge handles.
func FormatOf(ext string) (Format, bool) {
	switch Format(ext) {
	case JSON, TOML:
		return Format(ext), true
	}
	return "", false
}

// Fragment is one layer's contribution to a merged file.
type Fragment struct {
	// Data is the fragment's source bytes.
	Data []byte
	// Name identifies the fragment in errors, usually source:path.
	Name string
}

// variablePattern matches an escaped marker, $${, or a reference ${NAME}
// whose NAME is a letter or underscore followed by letters, digits, or
// underscores.
var variablePattern = regexp.MustCompile(`\$\$\{|\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Render parses every fragment as format, substitutes ${NAME} references in
// string values and map keys from lookupEnv, merges the fragments in order,
// and returns the serialized result.
//
// Maps merge key by key, lists concatenate with the later fragment's items
// last, and any other pairing takes the later value. A map, a list, or a
// plain value meeting a different kind at the same key is an error that
// names both fragments. A fragment that does not parse, whose root is not a
// map, that references an unset variable, or whose keys collide after
// substitution is an error naming the fragment. $${ writes a literal ${ and
// any other text is left as written.
func Render(format Format, fragments []Fragment, lookupEnv func(string) (string, bool)) ([]byte, error) {
	documents := make([]map[string]any, len(fragments))
	for i, fragment := range fragments {
		document, err := parse(format, fragment.Data)
		if err != nil {
			return nil, fmt.Errorf("fragment %s: %w", fragment.Name, err)
		}
		substituted, err := substituteMap(document, "", lookupEnv)
		if err != nil {
			return nil, fmt.Errorf("fragment %s: %w", fragment.Name, err)
		}
		documents[i] = substituted
	}

	merged := map[string]any{}
	for i, document := range documents {
		result, conflict := mergeMaps(merged, document, nil)
		if conflict != nil {
			earlier := fragments[lastHolder(documents[:i], conflict.path)].Name
			return nil, fmt.Errorf("fragment %s: key %s: cannot merge %s over %s from fragment %s",
				fragments[i].Name, strings.Join(conflict.path, "."), describe(format, conflict.over), describe(format, conflict.under), earlier)
		}
		merged = result
	}
	return encode(format, merged)
}

func parse(format Format, data []byte) (map[string]any, error) {
	switch format {
	case JSON:
		return parseJSON(data)
	case TOML:
		return parseTOML(data)
	}
	return nil, fmt.Errorf("unsupported format %q", format)
}

// parseJSON decodes one JSON object, keeping numbers as their source text so
// integers survive the round trip.
func parseJSON(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("parse JSON: data after the root object")
	}

	document, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse JSON: root is %s, not an object", describe(JSON, root))
	}
	return document, nil
}

// parseTOML decodes a TOML document and normalizes every array of tables to
// a plain list, so merging sees one list type.
func parseTOML(data []byte) (map[string]any, error) {
	var document map[string]any
	if _, err := toml.Decode(string(data), &document); err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	if document == nil {
		document = map[string]any{}
	}
	return normalizeTOML(document).(map[string]any), nil
}

func normalizeTOML(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = normalizeTOML(item)
		}
		return v
	case []map[string]any:
		list := make([]any, len(v))
		for i, item := range v {
			list[i] = normalizeTOML(item)
		}
		return list
	case []any:
		for i, item := range v {
			v[i] = normalizeTOML(item)
		}
		return v
	}
	return value
}

// substituteMap returns a copy of document with ${NAME} expanded in every key
// and string value. prefix is the path of document in the fragment followed
// by a dot, or empty at the root, and only names keys in error messages.
func substituteMap(document map[string]any, prefix string, lookupEnv func(string) (string, bool)) (map[string]any, error) {
	result := make(map[string]any, len(document))
	origins := make(map[string]string, len(document))
	for key, value := range document {
		keyPath := prefix + key
		expandedKey, err := expand(key, lookupEnv)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", keyPath, err)
		}
		if previous, ok := origins[expandedKey]; ok {
			return nil, fmt.Errorf("keys %s and %s both expand to %q", prefix+previous, keyPath, expandedKey)
		}
		origins[expandedKey] = key

		expandedValue, err := substituteValue(value, keyPath, lookupEnv)
		if err != nil {
			return nil, err
		}
		result[expandedKey] = expandedValue
	}
	return result, nil
}

// substituteValue expands ${NAME} in value, where path is the value's key
// path in the fragment, such as env.TOKEN or deny[1].
func substituteValue(value any, path string, lookupEnv func(string) (string, bool)) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		return substituteMap(v, path+".", lookupEnv)
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			expanded, err := substituteValue(item, path+"["+strconv.Itoa(i)+"]", lookupEnv)
			if err != nil {
				return nil, err
			}
			result[i] = expanded
		}
		return result, nil
	case string:
		expanded, err := expand(v, lookupEnv)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", path, err)
		}
		return expanded, nil
	}
	return value, nil
}

// expand replaces every ${NAME} in s with the variable's value and every $${
// with ${. An unset variable is an error.
func expand(s string, lookupEnv func(string) (string, bool)) (string, error) {
	var unset string
	expanded := variablePattern.ReplaceAllStringFunc(s, func(match string) string {
		if match == "$${" {
			return "${"
		}
		name := match[2 : len(match)-1]
		value, ok := lookupEnv(name)
		if !ok && unset == "" {
			unset = name
		}
		return value
	})
	if unset != "" {
		return "", fmt.Errorf("variable %s is not set", unset)
	}
	return expanded, nil
}

// kindConflict records two values of different kinds meeting at one key.
type kindConflict struct {
	over  any
	path  []string
	under any
}

// mergeMaps returns a new map holding under with over merged onto it, or the
// first kindConflict it meets.
func mergeMaps(under, over map[string]any, path []string) (map[string]any, *kindConflict) {
	result := make(map[string]any, len(under)+len(over))
	for key, value := range under {
		result[key] = value
	}
	for key, value := range over {
		existing, ok := under[key]
		if !ok {
			result[key] = value
			continue
		}
		merged, conflict := mergeValues(existing, value, append(slices.Clone(path), key))
		if conflict != nil {
			return nil, conflict
		}
		result[key] = merged
	}
	return result, nil
}

func mergeValues(under, over any, path []string) (any, *kindConflict) {
	underMap, underIsMap := under.(map[string]any)
	overMap, overIsMap := over.(map[string]any)
	underList, underIsList := under.([]any)
	overList, overIsList := over.([]any)
	switch {
	case underIsMap && overIsMap:
		return mergeMaps(underMap, overMap, path)
	case underIsList && overIsList:
		return append(append(make([]any, 0, len(underList)+len(overList)), underList...), overList...), nil
	case underIsMap || overIsMap || underIsList || overIsList:
		return nil, &kindConflict{over: over, path: path, under: under}
	}
	return over, nil
}

// lastHolder returns the index of the last document that holds a value at
// path. Callers pass the documents merged so far, so a conflict always has
// one.
func lastHolder(documents []map[string]any, path []string) int {
	for i := len(documents) - 1; i >= 0; i-- {
		if _, ok := valueAt(documents[i], path); ok {
			return i
		}
	}
	return 0
}

func valueAt(document map[string]any, path []string) (any, bool) {
	var current any = document
	for _, key := range path {
		table, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = table[key]; !ok {
			return nil, false
		}
	}
	return current, true
}

// describe names a value's kind in the format's own vocabulary.
func describe(format Format, value any) string {
	switch value.(type) {
	case map[string]any:
		if format == TOML {
			return "a table"
		}
		return "an object"
	case []any:
		return "an array"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case nil:
		return "null"
	case json.Number, int64, float64:
		return "a number"
	}
	return fmt.Sprintf("a %T", value)
}

func encode(format Format, document map[string]any) ([]byte, error) {
	var out bytes.Buffer
	switch format {
	case JSON:
		encoder := json.NewEncoder(&out)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(document); err != nil {
			return nil, fmt.Errorf("encode JSON: %w", err)
		}
	case TOML:
		encoder := toml.NewEncoder(&out)
		encoder.Indent = ""
		if err := encoder.Encode(document); err != nil {
			return nil, fmt.Errorf("encode TOML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
	return out.Bytes(), nil
}
