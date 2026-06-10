package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	yaml "go.yaml.in/yaml/v3"
)

const (
	yamlMapTag       = "!!map"
	yamlSeqTag       = "!!seq"
	yamlNullTag      = "!!null"
	yamlStringTag    = "!!str"
	yamlTimestampTag = "!!timestamp"
	yamlBoolTag      = "!!bool"
	yamlIntTag       = "!!int"
	yamlFloatTag     = "!!float"
)

func parseYAML(data []byte) (any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}, nil
		}
		return nil, err
	}

	var extra yaml.Node
	err := dec.Decode(&extra)
	if err == nil {
		return nil, fmt.Errorf("YAML streams with multiple documents are unsupported")
	}
	if !errors.Is(err, io.EOF) {
		return nil, err
	}

	return yamlDocumentToValue(&doc, "$")
}

func yamlDocumentRoot(n *yaml.Node) (*yaml.Node, error) {
	if n == nil || n.Kind == 0 {
		return nil, nil
	}
	if n.Kind != yaml.DocumentNode {
		return n, nil
	}
	switch len(n.Content) {
	case 0:
		return nil, nil
	case 1:
		return n.Content[0], nil
	default:
		return nil, fmt.Errorf("invalid YAML document: expected one root node, got %d", len(n.Content))
	}
}

func yamlEmptyRoot(n *yaml.Node) bool {
	if n == nil {
		return true
	}
	return n.Kind == yaml.ScalarNode && n.ShortTag() == yamlNullTag && n.Value == "" && n.Style == 0
}

func yamlNullRoot(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	return n.Kind == yaml.ScalarNode && n.ShortTag() == yamlNullTag
}

func yamlDocumentToValue(n *yaml.Node, path string) (any, error) {
	root, err := yamlDocumentRoot(n)
	if err != nil {
		return nil, err
	}
	if yamlEmptyRoot(root) {
		return map[string]any{}, nil
	}
	if yamlNullRoot(root) {
		return nil, fmt.Errorf("YAML root null is unsupported")
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("YAML root must be a mapping, got %s", yamlNodeKind(root))
	}
	return yamlNodeToValue(root, path)
}

func yamlNodeKind(n *yaml.Node) string {
	if n == nil {
		return "empty"
	}
	switch n.Kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("node kind %d", n.Kind)
	}
}

func yamlNodeToValue(n *yaml.Node, path string) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		return yamlDocumentToValue(n, path)
	case yaml.MappingNode:
		if tag := n.ShortTag(); tag != yamlMapTag {
			return nil, fmt.Errorf("unsupported YAML mapping tag %s at %s", tag, path)
		}
		return yamlMappingToValue(n, path)
	case yaml.SequenceNode:
		return yamlSequenceToValue(n, path)
	case yaml.ScalarNode:
		return yamlScalarToValue(n, path)
	case yaml.AliasNode:
		return nil, fmt.Errorf("YAML aliases are unsupported at %s", path)
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d at %s", n.Kind, path)
	}
}

func yamlMappingToValue(n *yaml.Node, path string) (map[string]any, error) {
	out := make(map[string]any, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("complex YAML mapping keys are unsupported at %s", path)
		}
		if tag := keyNode.ShortTag(); tag != yamlStringTag {
			return nil, fmt.Errorf("YAML mapping key at %s must be a string, got %s", path, tag)
		}
		key := keyNode.Value
		if _, ok := out[key]; ok {
			return nil, fmt.Errorf("duplicate YAML mapping key %q at %s", key, path)
		}

		v, err := yamlNodeToValue(n.Content[i+1], yamlPath(path, key))
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
	return out, nil
}

func yamlSequenceToValue(n *yaml.Node, path string) ([]any, error) {
	if tag := n.ShortTag(); tag != yamlSeqTag {
		return nil, fmt.Errorf("unsupported YAML sequence tag %s at %s", tag, path)
	}
	items := make([]any, len(n.Content))
	for i, item := range n.Content {
		v, err := yamlNodeToValue(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		items[i] = v
	}
	return items, nil
}

func yamlScalarToValue(n *yaml.Node, path string) (any, error) {
	tag := n.ShortTag()
	switch tag {
	case yamlNullTag:
		return nil, nil
	case yamlStringTag, yamlTimestampTag:
		return n.Value, nil
	case yamlBoolTag:
		var v bool
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	case yamlIntTag, yamlFloatTag:
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported YAML scalar tag %s at %s", tag, path)
	}
}

func yamlPath(parent, key string) string {
	quotedKey := fmt.Sprintf("%q", key)
	if parent == "$" {
		return "$[" + quotedKey + "]"
	}
	return parent + "[" + quotedKey + "]"
}

func serializeYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
