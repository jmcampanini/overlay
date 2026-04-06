package document

import (
	"reflect"
	"strings"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		name string
		want Format
	}{
		{"foo.json", FormatJSON},
		{"foo.toml", FormatTOML},
		{"foo.JSON", FormatJSON},
		{"path/to/config.toml", FormatTOML},
	}
	for _, tc := range cases {
		got, err := DetectFormat(tc.name)
		if err != nil {
			t.Errorf("DetectFormat(%q) error: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("DetectFormat(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDetectFormatUnknown(t *testing.T) {
	if _, err := DetectFormat("foo.yaml"); err == nil {
		t.Error("expected error for yaml")
	}
	if _, err := DetectFormat("foo"); err == nil {
		t.Error("expected error for no extension")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	input := []byte(`{"a":1,"b":{"c":"nested","d":[1,2,3]}}`)
	v, err := Parse(input, FormatJSON)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Serialize(v, FormatJSON)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	roundTripped, err := Parse(out, FormatJSON)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if !reflect.DeepEqual(v, roundTripped) {
		t.Errorf("round-trip mismatch:\noriginal: %v\nfinal:    %v", v, roundTripped)
	}
}

func TestJSONSerializeAlphabetizesKeys(t *testing.T) {
	v := map[string]any{"zebra": 1, "alpha": 2, "mango": 3}
	out, err := Serialize(v, FormatJSON)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	s := string(out)
	alphaIdx := strings.Index(s, "alpha")
	mangoIdx := strings.Index(s, "mango")
	zebraIdx := strings.Index(s, "zebra")
	if alphaIdx >= mangoIdx || mangoIdx >= zebraIdx {
		t.Errorf("keys not alphabetized:\n%s", s)
	}
}

func TestJSONSerializeNoHTMLEscape(t *testing.T) {
	v := map[string]any{"url": "https://example.com?a=1&b=2"}
	out, err := Serialize(v, FormatJSON)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if strings.Contains(string(out), `\u0026`) {
		t.Errorf("HTML-escaped output:\n%s", out)
	}
}

func TestTOMLRoundTrip(t *testing.T) {
	input := []byte(`
title = "test"

[section]
key = "value"
number = 42

[section.nested]
flag = true
`)
	v, err := Parse(input, FormatTOML)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Serialize(v, FormatTOML)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	roundTripped, err := Parse(out, FormatTOML)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if !reflect.DeepEqual(v, roundTripped) {
		t.Errorf("round-trip mismatch:\noriginal: %v\nfinal:    %v", v, roundTripped)
	}
}

func TestTOMLSerializeAlphabetizesKeys(t *testing.T) {
	v := map[string]any{
		"zebra": 1,
		"alpha": 2,
		"mango": 3,
	}
	out, err := Serialize(v, FormatTOML)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	s := string(out)
	alphaIdx := strings.Index(s, "alpha")
	mangoIdx := strings.Index(s, "mango")
	zebraIdx := strings.Index(s, "zebra")
	if alphaIdx >= mangoIdx || mangoIdx >= zebraIdx {
		t.Errorf("TOML keys not alphabetized:\n%s", s)
	}
}

func TestTOMLDottedKeyTables(t *testing.T) {
	input := []byte(`
[projects."/path/to/repo"]
trust_level = "trusted"
`)
	v, err := Parse(input, FormatTOML)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	projects, ok := v.(map[string]any)["projects"].(map[string]any)
	if !ok {
		t.Fatalf("projects not a map: %T", v.(map[string]any)["projects"])
	}
	repo, ok := projects["/path/to/repo"].(map[string]any)
	if !ok {
		t.Fatalf("repo not a map: %T", projects["/path/to/repo"])
	}
	if repo["trust_level"] != "trusted" {
		t.Errorf("trust_level = %v", repo["trust_level"])
	}
}

func TestJSONObjectArrays(t *testing.T) {
	input := []byte(`{"hooks":[{"type":"command","command":"a.sh"},{"type":"command","command":"b.sh"}]}`)
	v, err := Parse(input, FormatJSON)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	hooks, ok := v.(map[string]any)["hooks"].([]any)
	if !ok {
		t.Fatalf("hooks not a list: %T", v.(map[string]any)["hooks"])
	}
	if len(hooks) != 2 {
		t.Errorf("got %d hooks, want 2", len(hooks))
	}
}
