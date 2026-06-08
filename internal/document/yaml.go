package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	yaml "go.yaml.in/yaml/v3"
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
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("YAML streams with multiple documents are unsupported")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}

	return yamlDocumentToValue(&doc, "$")
}

func yamlDocumentRoot(n *yaml.Node) (*yaml.Node, error) {
	if n.Kind == 0 {
		return nil, nil
	}
	if n.Kind != yaml.DocumentNode {
		return n, nil
	}
	if len(n.Content) == 0 {
		return nil, nil
	}
	if len(n.Content) != 1 {
		return nil, fmt.Errorf("invalid YAML document: expected one root node, got %d", len(n.Content))
	}
	return n.Content[0], nil
}

func yamlEmptyRoot(n *yaml.Node) bool {
	if n == nil {
		return true
	}
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null" && n.Value == "" && n.Style == 0
}

func yamlNullRoot(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
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
	return yamlNodeToValue(root, path)
}

func yamlNodeToValue(n *yaml.Node, path string) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		return yamlDocumentToValue(n, path)
	case yaml.MappingNode:
		if n.ShortTag() != "!!map" {
			return nil, fmt.Errorf("unsupported YAML mapping tag %s at %s", n.ShortTag(), path)
		}
		return yamlMappingToValue(n, path)
	case yaml.SequenceNode:
		if n.ShortTag() != "!!seq" {
			return nil, fmt.Errorf("unsupported YAML sequence tag %s at %s", n.ShortTag(), path)
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
	seen := make(map[string]struct{}, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("complex YAML mapping keys are unsupported at %s", path)
		}
		if keyNode.ShortTag() != "!!str" {
			return nil, fmt.Errorf("YAML mapping key at %s must be a string, got %s", path, keyNode.ShortTag())
		}
		key := keyNode.Value
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate YAML mapping key %q at %s", key, path)
		}
		seen[key] = struct{}{}

		v, err := yamlNodeToValue(n.Content[i+1], yamlPath(path, key))
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
	return out, nil
}

func yamlScalarToValue(n *yaml.Node, path string) (any, error) {
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!str", "!!timestamp":
		return n.Value, nil
	case "!!bool":
		var v bool
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	case "!!int", "!!float":
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported YAML scalar tag %s at %s", n.ShortTag(), path)
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
