// Package merge composes the layered fragments of one JSON, TOML, or YAML
// file into a single document. Each fragment is parsed on its own, the
// fragments deep-merge in the order given, and the result serializes with
// sorted keys.
package merge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"
)

// Format is the document language of a set of fragments.
type Format string

const (
	// JSON fragments parse as JSON objects and serialize with two-space
	// indentation.
	JSON Format = "json"
	// TOML fragments parse as TOML tables and serialize flush-left.
	TOML Format = "toml"
	// YAML fragments parse as YAML mappings and serialize in block style with
	// two-space indentation.
	YAML Format = "yaml"
)

// FormatOf returns the Format named by a file extension without its dot, and
// false when the extension is not one merge handles. Both yaml and yml name
// YAML.
func FormatOf(ext string) (Format, bool) {
	switch ext {
	case "json":
		return JSON, true
	case "toml":
		return TOML, true
	case "yaml", "yml":
		return YAML, true
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

// Document is one parsed fragment: its root table and the name errors use.
type Document struct {
	Name  string
	Table map[string]any
}

// Render parses every fragment as format with Parse, merges the documents
// with Merge, and returns the result encoded with Encode.
func Render(format Format, fragments []Fragment) ([]byte, error) {
	documents := make([]Document, len(fragments))
	for i, fragment := range fragments {
		document, err := Parse(format, fragment)
		if err != nil {
			return nil, err
		}
		documents[i] = document
	}

	merged, err := Merge(format, documents)
	if err != nil {
		return nil, err
	}
	return Encode(format, merged)
}

// Parse parses fragment as format. A fragment that does not parse or whose
// root is not a map is an error naming the fragment. A fragment that is
// empty or only whitespace is an empty document in every format.
//
// JSON numbers and YAML numbers and timestamps keep their source text. YAML
// aliases expand in place; a YAML fragment with a merge key, a key that is
// not a string, a tag outside the core schema, or a second document is an
// error, and a comment-only one is an empty mapping.
func Parse(format Format, fragment Fragment) (Document, error) {
	table, err := parse(format, fragment.Data)
	if err != nil {
		return Document{}, fmt.Errorf("fragment %s: %w", fragment.Name, err)
	}
	return Document{Name: fragment.Name, Table: table}, nil
}

// Merge deep-merges documents in order and returns a new table. Maps merge
// key by key, lists concatenate with the later document's items last, and
// any other pairing takes the later value. A map, a list, or a plain value
// meeting a different kind at the same key is an error that names both
// documents in format's vocabulary.
func Merge(format Format, documents []Document) (map[string]any, error) {
	merged := map[string]any{}
	for i, document := range documents {
		result, conflict := mergeMaps(merged, document.Table, nil)
		if conflict != nil {
			earlier := documents[lastHolder(documents[:i], conflict.path)].Name
			return nil, fmt.Errorf("fragment %s: key %s: cannot merge %s over %s from fragment %s",
				document.Name, strings.Join(conflict.path, "."), describe(format, conflict.over), describe(format, conflict.under), earlier)
		}
		merged = result
	}
	return merged, nil
}

func parse(format Format, data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}

	switch format {
	case JSON:
		return parseJSON(data)
	case TOML:
		return parseTOML(data)
	case YAML:
		return parseYAML(data)
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

// yamlScalar is a YAML number or timestamp kept as its source text, so 1.10
// and 0x1F serialize as written. tag is the resolved short tag, such as
// !!float.
type yamlScalar struct {
	tag  string
	text string
}

// yamlQuotedWords are strings the encoder must double-quote: the YAML 1.1
// boolean words that YAML 1.2 reads as strings, so readers of either
// version see a string, and the merge key marker, which the encoder would
// otherwise write plain and a reader would take as a merge.
var yamlQuotedWords = map[string]bool{
	"y": true, "Y": true, "yes": true, "Yes": true, "YES": true,
	"n": true, "N": true, "no": true, "No": true, "NO": true,
	"on": true, "On": true, "ON": true, "off": true, "Off": true, "OFF": true,
	"<<": true,
}

// parseYAML decodes one YAML document through its node tree and lowers it
// to the generic shape merging uses, keeping number and timestamp text.
func parseYAML(data []byte) (map[string]any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	err := decoder.Decode(&document)
	if errors.Is(err, io.EOF) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	err = decoder.Decode(&yaml.Node{})
	if err == nil {
		return nil, errors.New("parse YAML: a second document after the root mapping")
	}
	if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	if len(document.Content) == 0 || isBareDocument(document.Content[0]) {
		return map[string]any{}, nil
	}

	root, err := lowerYAML(document.Content[0], map[*yaml.Node]bool{})
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	mapping, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse YAML: root is %s, not a mapping", describe(YAML, root))
	}
	return mapping, nil
}

// isBareDocument reports whether node is the untagged empty null a document
// marker with nothing after it yields, as opposed to an explicit ~, null,
// or !!null.
func isBareDocument(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null" && node.Value == "" && node.Style == 0
}

// lowerYAML converts a node to maps, lists, strings, booleans, nil, and
// yamlScalar values. expanding holds the anchored nodes whose aliases are
// being followed, so an alias inside its own anchor is an error instead of
// a loop.
func lowerYAML(node *yaml.Node, expanding map[*yaml.Node]bool) (any, error) {
	switch node.Kind {
	case yaml.AliasNode:
		if expanding[node.Alias] {
			return nil, fmt.Errorf("line %d: alias *%s is inside the node it refers to", node.Line, node.Value)
		}
		expanding[node.Alias] = true
		defer delete(expanding, node.Alias)
		return lowerYAML(node.Alias, expanding)
	case yaml.MappingNode:
		if node.Tag != "!!map" {
			return nil, fmt.Errorf("line %d: tag %s is not supported", node.Line, node.Tag)
		}
		mapping := make(map[string]any, len(node.Content)/2)
		lines := make(map[string]int, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, err := lowerYAMLKey(node.Content[i])
			if err != nil {
				return nil, err
			}
			if line, seen := lines[key]; seen {
				return nil, fmt.Errorf("line %d: key %s already defined at line %d", node.Content[i].Line, key, line)
			}
			lines[key] = node.Content[i].Line
			value, err := lowerYAML(node.Content[i+1], expanding)
			if err != nil {
				return nil, err
			}
			mapping[key] = value
		}
		return mapping, nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return nil, fmt.Errorf("line %d: tag %s is not supported", node.Line, node.Tag)
		}
		list := make([]any, len(node.Content))
		for i, item := range node.Content {
			value, err := lowerYAML(item, expanding)
			if err != nil {
				return nil, err
			}
			list[i] = value
		}
		return list, nil
	case yaml.ScalarNode:
		return lowerYAMLScalar(node)
	}
	return nil, fmt.Errorf("line %d: unexpected node kind %d", node.Line, node.Kind)
}

// lowerYAMLKey returns the string a mapping key holds, following an alias
// to its anchored scalar.
func lowerYAMLKey(node *yaml.Node) (string, error) {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("line %d: key is not a string", node.Line)
	}
	switch node.Tag {
	case "!!str":
		return node.Value, nil
	case "!!merge":
		return "", fmt.Errorf("line %d: merge key << is not supported; layers merge mappings instead", node.Line)
	}
	return "", fmt.Errorf("line %d: key %s is %s, not a string", node.Line, node.Value, describeYAMLTag(node.Tag))
}

// lowerYAMLScalar converts a scalar by its tag, decoding it first so an
// explicit tag with text that is not a value of that type, such as !!int
// garbage, is rejected by the library's own rules.
func lowerYAMLScalar(node *yaml.Node) (any, error) {
	switch node.Tag {
	case "!!str", "!!null", "!!bool", "!!int", "!!float", "!!timestamp":
	default:
		return nil, fmt.Errorf("line %d: tag %s is not supported", node.Line, node.Tag)
	}
	var decoded any
	if err := node.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("line %d: %w", node.Line, err)
	}

	switch node.Tag {
	case "!!str":
		return node.Value, nil
	case "!!null", "!!bool":
		return decoded, nil
	}
	return yamlScalar{tag: node.Tag, text: node.Value}, nil
}

func describeYAMLTag(tag string) string {
	switch tag {
	case "!!int", "!!float":
		return "a number"
	case "!!timestamp":
		return "a timestamp"
	case "!!bool":
		return "a boolean"
	case "!!null":
		return "null"
	}
	return "tagged " + tag
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
func lastHolder(documents []Document, path []string) int {
	for i := len(documents) - 1; i >= 0; i-- {
		if _, ok := valueAt(documents[i].Table, path); ok {
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
	switch v := value.(type) {
	case map[string]any:
		switch format {
		case TOML:
			return "a table"
		case YAML:
			return "a mapping"
		}
		return "an object"
	case []any:
		if format == YAML {
			return "a sequence"
		}
		return "an array"
	case yamlScalar:
		return describeYAMLTag(v.tag)
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

// Encode serializes document as format with keys sorted bytewise at every
// level: JSON indented with two spaces, TOML flush-left, and YAML in block
// style with two-space indentation.
func Encode(format Format, document map[string]any) ([]byte, error) {
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
	case YAML:
		encoder := yaml.NewEncoder(&out)
		encoder.SetIndent(2)
		if err := encoder.Encode(yamlNode(document)); err != nil {
			return nil, fmt.Errorf("encode YAML: %w", err)
		}
		if err := encoder.Close(); err != nil {
			return nil, fmt.Errorf("encode YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
	return out.Bytes(), nil
}

// yamlNode builds the node tree the YAML encoder writes, with mapping keys
// sorted bytewise and scalars tagged so numbers and timestamps print as
// written and strings that would read as another type get quoted.
func yamlNode(value any) *yaml.Node {
	switch v := value.(type) {
	case map[string]any:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, key := range slices.Sorted(maps.Keys(v)) {
			node.Content = append(node.Content, yamlString(key), yamlNode(v[key]))
		}
		return node
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range v {
			node.Content = append(node.Content, yamlNode(item))
		}
		return node
	case string:
		return yamlString(v)
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case yamlScalar:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: v.tag, Value: v.text}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprint(value)}
}

func yamlString(s string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if yamlQuotedWords[s] {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}
