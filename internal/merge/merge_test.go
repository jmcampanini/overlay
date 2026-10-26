package merge

import (
	"strings"
	"testing"
)

func lookupFrom(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := vars[name]
		return value, ok
	}
}

func noEnv(string) (string, bool) { return "", false }

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

	got, err := Render(JSON, fragments, noEnv)
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

	got, err := Render(TOML, fragments, noEnv)
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
	got, err := Render(JSON, []Fragment{{Name: "only", Data: []byte(`{"b":1,"a":{"d":true,"c":null}}`)}}, noEnv)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "{\n  \"a\": {\n    \"c\": null,\n    \"d\": true\n  },\n  \"b\": 1\n}\n"
	if string(got) != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderSubstitutesVariablesInValuesAndKeys(t *testing.T) {
	lookup := lookupFrom(map[string]string{"HOME": "/home/me", "EMPTY": "", "N": "7"})
	fragments := []Fragment{{Name: "f", Data: []byte(`
plans = "${HOME}/plans"
literal = "$${HOME} stays, $HOME too, ${lower-case} too"
empty = "<${EMPTY}>"
number = "${N}"
list = ["${HOME}", 1]

[projects."${HOME}/code"]
trust = "${HOME}"
`)}}

	got, err := Render(TOML, fragments, lookup)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := `empty = "<>"
list = ["/home/me", 1]
literal = "${HOME} stays, $HOME too, ${lower-case} too"
number = "7"
plans = "/home/me/plans"

[projects]
[projects."/home/me/code"]
trust = "/home/me"
`
	if string(got) != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
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
		{name: "empty JSON", format: JSON, fragments: []Fragment{{Name: "s:a", Data: nil}},
			want: []string{"parse JSON"}},
		{name: "trailing JSON", format: JSON, fragments: []Fragment{{Name: "s:a", Data: []byte(`{} {}`)}},
			want: []string{"data after the root object"}},
		{name: "invalid TOML", format: TOML, fragments: []Fragment{{Name: "s:b", Data: []byte("a = \n")}},
			want: []string{"fragment s:b", "parse TOML"}},
		{name: "unset variable names the key", format: JSON, fragments: []Fragment{{Name: "s:a", Data: []byte(`{"env": {"TOKEN": "${OVERLAY_TEST_UNSET}"}}`)}},
			want: []string{"fragment s:a", "key env.TOKEN", "variable OVERLAY_TEST_UNSET is not set"}},
		{name: "unset variable inside a list", format: JSON, fragments: []Fragment{{Name: "s:a", Data: []byte(`{"deny": ["ok", "${OVERLAY_TEST_UNSET}"]}`)}},
			want: []string{"key deny[1]"}},
		{name: "keys collide after substitution", format: TOML, fragments: []Fragment{{Name: "s:a", Data: []byte("[t]\n\"${X}\" = 1\nv = 2\n")}},
			want: []string{"both expand to \"v\""}},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := lookupFrom(map[string]string{"X": "v"})

			_, err := Render(tt.format, tt.fragments, lookup)

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

	got, err := Render(JSON, fragments, noEnv)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "{\n  \"a\": \"one\",\n  \"b\": null,\n  \"c\": 2.5\n}\n"
	if string(got) != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderEmptyTOMLFragmentsProduceAnEmptyFile(t *testing.T) {
	got, err := Render(TOML, []Fragment{{Name: "a", Data: []byte("# only a comment\n")}, {Name: "b", Data: nil}}, noEnv)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(got) != 0 {
		t.Errorf("Render() = %q, want no bytes", got)
	}
}
