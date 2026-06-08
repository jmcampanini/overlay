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
		{"foo.yaml", FormatYAML},
		{"foo.yml", FormatYAML},
		{"foo.JSON", FormatJSON},
		{"path/to/config.YML", FormatYAML},
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

func TestTOMLSerializeDoesNotIndentTablesByDefault(t *testing.T) {
	v := map[string]any{
		"actions": []any{map[string]any{"name": "build", "run": "make build"}},
		"projects": map[string]any{
			"/path/to/repo": map[string]any{"trust_level": "trusted"},
		},
	}
	out, err := Serialize(v, FormatTOML)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"[[actions]]\nname = 'build'\nrun = 'make build'",
		"[projects]\n[projects.'/path/to/repo']\ntrust_level = 'trusted'",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing unindented TOML block %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\n  name =") || strings.Contains(s, "\n  [projects.") {
		t.Errorf("TOML tables should not be indented by default:\n%s", s)
	}
}

func TestTOMLSerializeCanIndentTables(t *testing.T) {
	v := map[string]any{
		"actions": []any{map[string]any{"name": "build", "run": "make build"}},
		"projects": map[string]any{
			"/path/to/repo": map[string]any{"trust_level": "trusted"},
		},
	}
	out, err := SerializeWithOptions(v, FormatTOML, SerializeOptions{TOMLIndentTables: true})
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"[[actions]]\n  name = 'build'\n  run = 'make build'",
		"[projects]\n  [projects.'/path/to/repo']\n    trust_level = 'trusted'",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing indented TOML block %q:\n%s", want, s)
		}
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

func TestYAMLRoundTrip(t *testing.T) {
	input := []byte(`app:
  name: overlay
  features:
    - json
    - yaml
  debug: true
`)
	v, err := Parse(input, FormatYAML)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Serialize(v, FormatYAML)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	roundTripped, err := Parse(out, FormatYAML)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if !reflect.DeepEqual(v, roundTripped) {
		t.Errorf("round-trip mismatch:\noriginal: %v\nfinal:    %v", v, roundTripped)
	}
}

func TestYAMLSerializeAlphabetizesKeys(t *testing.T) {
	v := map[string]any{"zebra": 1, "alpha": 2, "mango": 3}
	out, err := Serialize(v, FormatYAML)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	s := string(out)
	alphaIdx := strings.Index(s, "alpha")
	mangoIdx := strings.Index(s, "mango")
	zebraIdx := strings.Index(s, "zebra")
	if alphaIdx >= mangoIdx || mangoIdx >= zebraIdx {
		t.Errorf("YAML keys not alphabetized:\n%s", s)
	}
}

func TestYAMLSerializeBlockStyleWithTwoSpaceIndent(t *testing.T) {
	v := map[string]any{
		"app": map[string]any{
			"features": []any{"json", "yaml"},
		},
	}
	out, err := Serialize(v, FormatYAML)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := "app:\n  features:\n    - json\n    - yaml\n"
	if string(out) != want {
		t.Errorf("YAML output mismatch:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestYAMLRejectsMultipleDocuments(t *testing.T) {
	_, err := Parse([]byte("a: 1\n---\nb: 2\n"), FormatYAML)
	if err == nil || !strings.Contains(err.Error(), "multiple documents") {
		t.Fatalf("Parse error = %v, want multiple documents", err)
	}
}

func TestYAMLRejectsNonStringMappingKeys(t *testing.T) {
	_, err := Parse([]byte("1: one\n"), FormatYAML)
	if err == nil || !strings.Contains(err.Error(), "must be a string") {
		t.Fatalf("Parse error = %v, want string-key error", err)
	}
}

func TestYAMLRejectsComplexMappingKeys(t *testing.T) {
	_, err := Parse([]byte("? [a, b]\n: c\n"), FormatYAML)
	if err == nil || !strings.Contains(err.Error(), "complex YAML mapping keys") {
		t.Fatalf("Parse error = %v, want complex-key error", err)
	}
}

func TestYAMLRejectsInvalidSyntax(t *testing.T) {
	_, err := Parse([]byte("a: [1,\n"), FormatYAML)
	if err == nil {
		t.Fatal("expected invalid YAML error")
	}
}

func TestYAMLRejectsCustomTags(t *testing.T) {
	_, err := Parse([]byte("value: !secret token\n"), FormatYAML)
	if err == nil || !strings.Contains(err.Error(), "unsupported YAML scalar tag") {
		t.Fatalf("Parse error = %v, want custom-tag error", err)
	}
}
