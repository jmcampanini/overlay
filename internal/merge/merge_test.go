package merge

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRenderJSONDeepMergesAndConcatenatesLists(t *testing.T) {
	fragments := []Fragment{
		{Name: "base", Data: []byte(`{
  "permissions": {"deny": ["Read(a)", "Read(b)"], "defaultMode": "ask"},
  "hooks": {"SessionStart": [{"hooks": [{"command": "one"}]}]},
  "model": "opus",
  "zeta": 1.0,
  "big": 12345678901234567890
}`)},
		{Name: "personal", Data: []byte(`{
  "permissions": {"deny": ["Read(b)"]},
  "hooks": {"SessionStart": [{"hooks": [{"command": "two"}]}], "SessionEnd": []},
  "model": "sonnet",
  "env": {"URL": "http://x/<a>&b"}
}`)},
	}

	got, err := Render(JSON, fragments)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := `{
  "big": 12345678901234567890,
  "env": {
    "URL": "http://x/<a>&b"
  },
  "hooks": {
    "SessionEnd": [],
    "SessionStart": [
      {
        "hooks": [
          {
            "command": "one"
          }
        ]
      },
      {
        "hooks": [
          {
            "command": "two"
          }
        ]
      }
    ]
  },
  "model": "sonnet",
  "permissions": {
    "defaultMode": "ask",
    "deny": [
      "Read(a)",
      "Read(b)",
      "Read(b)"
    ]
  },
  "zeta": 1.0
}
`
	if string(got) != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderTOMLMergesTablesAndArraysOfTables(t *testing.T) {
	fragments := []Fragment{
		{Name: "base", Data: []byte(`model = "gpt"
approval_policy = "on-request"

[features]
apps = false

[[hooks.PreToolUse]]
matcher = "^a$"

[projects."/home/x/one"]
trust_level = "trusted"
`)},
		{Name: "personal", Data: []byte(`model = "gpt-pro"

[features]
goals = true

[[hooks.PreToolUse]]
matcher = "^b$"

[projects."/home/x/two"]
trust_level = "trusted"
`)},
	}

	got, err := Render(TOML, fragments)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := `approval_policy = "on-request"
model = "gpt-pro"

[features]
apps = false
goals = true

[hooks]

[[hooks.PreToolUse]]
matcher = "^a$"

[[hooks.PreToolUse]]
matcher = "^b$"

[projects]
[projects."/home/x/one"]
trust_level = "trusted"
[projects."/home/x/two"]
trust_level = "trusted"
`
	if string(got) != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderSingleFragmentIsNormalized(t *testing.T) {
	got, err := Render(JSON, []Fragment{{Name: "only", Data: []byte(`{"b":1,"a":{"d":true,"c":null}}`)}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "{\n  \"a\": {\n    \"c\": null,\n    \"d\": true\n  },\n  \"b\": 1\n}\n"
	if string(got) != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderYAMLMergesMappingsAndKeepsScalarText(t *testing.T) {
	fragments := []Fragment{
		{Name: "base", Data: []byte(`# base settings
version: 1.10
mask: 0x1F
since: 2001-12-14
enabled: yes
quoted: "1.0"
nothing: ~
flag: true
defaults: &shared
  retries: 3
servers:
  - name: alpha
    tags: [a, b]
    options: *shared
hooks: {}
b10: 1
B1: 2
`)},
		{Name: "personal", Data: []byte(`
servers:
  - name: beta
b2: 3
path: /home/me/plans
note: |
  first line
  second line
hooks:
  start:
    - echo hi
`)},
	}

	got, err := Render(YAML, fragments)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := `B1: 2
b10: 1
b2: 3
defaults:
  retries: 3
enabled: "yes"
flag: true
hooks:
  start:
    - echo hi
mask: 0x1F
note: |
  first line
  second line
nothing: null
path: /home/me/plans
quoted: "1.0"
servers:
  - name: alpha
    options:
      retries: 3
    tags:
      - a
      - b
  - name: beta
since: 2001-12-14
version: 1.10
`
	if string(got) != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderYAMLQuotesStringsThatWouldReadAsAnotherType(t *testing.T) {
	fragments := []Fragment{{Name: "f", Data: []byte(`
"on": "true"
"off": "null"
date: "2001-12-14"
number: "0x1F"
dash: "- item"
hash: "#comment"
empty: ""
`)}}

	got, err := Render(YAML, fragments)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var back map[string]any
	if err := yaml.Unmarshal(got, &back); err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, got)
	}
	for key, value := range back {
		if _, ok := value.(string); !ok {
			t.Errorf("key %s reads back as %T, want string:\n%s", key, value, got)
		}
	}
	if !strings.Contains(string(got), `"on": "true"`) {
		t.Errorf("Render() =\n%s\nwant the YAML 1.1 boolean words double-quoted", got)
	}
}

func TestRenderYAMLKeepsAliasKeysAndLiteralMergeMarkers(t *testing.T) {
	fragments := []Fragment{{Name: "f", Data: []byte(`
name: &key hello
map:
  *key : value
  "<<": {b: 1}
marker: "<<"
`)}}

	got, err := Render(YAML, fragments)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "map:\n  \"<<\":\n    b: 1\n  hello: value\nmarker: \"<<\"\nname: hello\n"
	if string(got) != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
	again, err := Render(YAML, []Fragment{{Name: "again", Data: got}})
	if err != nil || string(again) != want {
		t.Errorf("Render() of its own output = %q (%v), want the same document", again, err)
	}
}

func TestRenderEmptyYAMLFragmentsProduceAnEmptyMapping(t *testing.T) {
	got, err := Render(YAML, []Fragment{{Name: "a", Data: []byte("# only a comment\n")}, {Name: "b", Data: nil}, {Name: "c", Data: []byte("---\n")}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(got) != "{}\n" {
		t.Errorf("Render() = %q, want %q", got, "{}\n")
	}
}

func TestRenderFailures(t *testing.T) {
	tests := []struct {
		name      string
		format    Format
		fragments []Fragment
		want      []string
	}{
		{name: "invalid JSON", format: JSON, fragments: []Fragment{{Name: "s:a.olay.base.json", Data: []byte(`{"a": }`)}},
			want: []string{"fragment s:a.olay.base.json", "parse JSON"}},
		{name: "JSON root is an array", format: JSON, fragments: []Fragment{{Name: "s:a", Data: []byte(`[1]`)}},
			want: []string{"root is an array, not an object"}},
		{name: "trailing JSON", format: JSON, fragments: []Fragment{{Name: "s:a", Data: []byte(`{} {}`)}},
			want: []string{"data after the root object"}},
		{name: "invalid TOML", format: TOML, fragments: []Fragment{{Name: "s:b", Data: []byte("a = \n")}},
			want: []string{"fragment s:b", "parse TOML"}},
		{name: "table over string", format: TOML, fragments: []Fragment{
			{Name: "s:base", Data: []byte("[a]\nb = \"text\"\n")},
			{Name: "s:mid", Data: []byte("other = 1\n")},
			{Name: "s:top", Data: []byte("[a.b]\nc = 1\n")},
		}, want: []string{"fragment s:top: key a.b: cannot merge a table over a string from fragment s:base"}},
		{name: "array over object", format: JSON, fragments: []Fragment{
			{Name: "s:base", Data: []byte(`{"hooks": {"a": 1}}`)},
			{Name: "s:top", Data: []byte(`{"hooks": []}`)},
		}, want: []string{"key hooks: cannot merge an array over an object from fragment s:base"}},
		{name: "null over object", format: JSON, fragments: []Fragment{
			{Name: "s:base", Data: []byte(`{"hooks": {"a": 1}}`)},
			{Name: "s:top", Data: []byte(`{"hooks": null}`)},
		}, want: []string{"cannot merge null over an object"}},
		{name: "invalid YAML", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: [1\n")}},
			want: []string{"fragment s:c", "parse YAML"}},
		{name: "YAML root is a sequence", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("- 1\n")}},
			want: []string{"root is a sequence, not a mapping"}},
		{name: "YAML root is null", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("~\n")}},
			want: []string{"root is null, not a mapping"}},
		{name: "second YAML document", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: 1\n---\nb: 2\n")}},
			want: []string{"a second document after the root mapping"}},
		{name: "YAML merge key", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("base: &b\n  x: 1\nover:\n  <<: *b\n")}},
			want: []string{"line 4: merge key << is not supported"}},
		{name: "YAML key is a number", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: 1\n10: b\n")}},
			want: []string{"line 2: key 10 is a number, not a string"}},
		{name: "YAML key is a sequence", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("? [x]\n: 1\n")}},
			want: []string{"line 1: key is not a string"}},
		{name: "YAML custom tag", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: !custom x\n")}},
			want: []string{"tag !custom is not supported"}},
		{name: "YAML custom tag on a mapping", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: !custom {b: 1}\n")}},
			want: []string{"line 1: tag !custom is not supported"}},
		{name: "YAML set tag on a sequence", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a:\n  - 1\n  - !!set [2]\n")}},
			want: []string{"line 3: tag !!set is not supported"}},
		{name: "YAML explicit int tag with text", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: !!int garbage\n")}},
			want: []string{"line 1:", "cannot decode !!str `garbage` as a !!int"}},
		{name: "YAML explicit null tag with text", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: !!null garbage\n")}},
			want: []string{"cannot decode !!str `garbage` as a !!null"}},
		{name: "YAML explicit bool tag with a number", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: !!bool 1\n")}},
			want: []string{"cannot decode !!int `1` as a !!bool"}},
		{name: "YAML explicit null root", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("--- !!null\n")}},
			want: []string{"root is null, not a mapping"}},
		{name: "YAML binary tag", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: !!binary aGk=\n")}},
			want: []string{"tag !!binary is not supported"}},
		{name: "YAML duplicate key", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: 1\nb:\n  c: 1\n  c: 2\n")}},
			want: []string{"line 4: key c already defined at line 3"}},
		{name: "YAML alias from another fragment", format: YAML, fragments: []Fragment{
			{Name: "s:base", Data: []byte("a: &x 1\n")},
			{Name: "s:top", Data: []byte("b: *x\n")},
		}, want: []string{"fragment s:top", "unknown anchor"}},
		{name: "YAML alias inside its own anchor", format: YAML, fragments: []Fragment{{Name: "s:c", Data: []byte("a: &x\n  b: *x\n")}},
			want: []string{"alias *x is inside the node it refers to"}},
		{name: "sequence over mapping", format: YAML, fragments: []Fragment{
			{Name: "s:base", Data: []byte("hooks:\n  a: 1\n")},
			{Name: "s:top", Data: []byte("hooks: [1]\n")},
		}, want: []string{"key hooks: cannot merge a sequence over a mapping from fragment s:base"}},
		{name: "timestamp over mapping", format: YAML, fragments: []Fragment{
			{Name: "s:base", Data: []byte("when:\n  a: 1\n")},
			{Name: "s:top", Data: []byte("when: 2001-12-14\n")},
		}, want: []string{"cannot merge a timestamp over a mapping"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.format, tt.fragments)

			if err == nil {
				t.Fatalf("Render() error = nil, want containing %q", tt.want)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Render() error = %q, want containing %q", err, want)
				}
			}
		})
	}
}

func TestRenderScalarOverScalarTakesTheLaterValue(t *testing.T) {
	fragments := []Fragment{
		{Name: "base", Data: []byte(`{"a": 1, "b": "s", "c": true}`)},
		{Name: "top", Data: []byte(`{"a": "one", "b": null, "c": 2.5}`)},
	}

	got, err := Render(JSON, fragments)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "{\n  \"a\": \"one\",\n  \"b\": null,\n  \"c\": 2.5\n}\n"
	if string(got) != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderWhitespaceOnlyFragmentsAreEmptyDocuments(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		value  string
		want   string
	}{
		{name: "JSON", format: JSON, value: `{"a": 1}`, want: "{\n  \"a\": 1\n}\n"},
		{name: "TOML", format: TOML, value: "a = 1\n", want: "a = 1\n"},
		{name: "YAML", format: YAML, value: "a: 1\n", want: "a: 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragments := []Fragment{
				{Name: "nothing", Data: nil},
				{Name: "whitespace", Data: []byte(" \n\t\n")},
				{Name: "value", Data: []byte(tt.value)},
			}

			got, err := Render(tt.format, fragments)

			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderEmptyTOMLFragmentsProduceAnEmptyFile(t *testing.T) {
	got, err := Render(TOML, []Fragment{{Name: "a", Data: []byte("# only a comment\n")}, {Name: "b", Data: nil}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(got) != 0 {
		t.Errorf("Render() = %q, want no bytes", got)
	}
}
