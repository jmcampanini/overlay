package document

import (
	"bytes"

	"github.com/pelletier/go-toml/v2"
)

func parseTOML(data []byte) (any, error) {
	var v map[string]any
	if err := toml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func serializeTOML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
