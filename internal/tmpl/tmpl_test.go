package tmpl

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func testContext(t *testing.T) Context {
	t.Helper()
	return Context{
		Files: func(patterns []string) ([]string, error) {
			var paths []string
			for _, pattern := range patterns {
				paths = append(paths, "/src/"+strings.ReplaceAll(pattern, "{layer}", "base"))
			}
			return paths, nil
		},
		Home:     "/home/me",
		Hostname: "wintermute.local",
		Layers:   []string{"base", "personal", "local"},
		OS:       "linux",
		Sources: []Source{
			{Name: "dotfiles-base", Path: filepath.Join(t.TempDir(), "base")},
			{Name: "dotfiles-personal", Path: filepath.Join(t.TempDir(), "personal")},
		},
	}
}

func TestRenderExposesEveryContextField(t *testing.T) {
	ctx := testContext(t)
	text := `home={{ .Home }}
hostname={{ .Hostname }}
layers={{ range .Layers }}{{ . }};{{ end }}
os={{ .OS }}
sources={{ range .Sources }}{{ .Name }}={{ .Path }};{{ end }}
`

	got, err := Render("fields", []byte(text), ctx)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "home=/home/me\nhostname=wintermute.local\nlayers=base;personal;local;\nos=linux\nsources=" +
		"dotfiles-base=" + ctx.Sources[0].Path + ";dotfiles-personal=" + ctx.Sources[1].Path + ";\n"
	if string(got) != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderFunctions(t *testing.T) {
	present := filepath.Join(t.TempDir(), "present")
	if err := os.WriteFile(present, []byte("x\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	multiline := filepath.Join(t.TempDir(), "multiline")
	if err := os.WriteFile(multiline, []byte("one\ntwo\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t)
	ctx.Values = map[string]any{"paths": map[string]any{"code": "/home/me/code"}}
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(present, link); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(t.TempDir(), "dangling")
	if err := os.Symlink(filepath.Join(dir, "gone"), dangling); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "fileExists regular file", text: `{{ fileExists "` + present + `" }}`, want: "true"},
		{name: "fileExists directory", text: `{{ fileExists "` + dir + `" }}`, want: "true"},
		{name: "fileExists symlink to file", text: `{{ fileExists "` + link + `" }}`, want: "true"},
		{name: "fileExists dangling symlink", text: `{{ fileExists "` + dangling + `" }}`, want: "false"},
		{name: "fileExists absent", text: `{{ fileExists "` + filepath.Join(dir, "missing") + `" }}`, want: "false"},
		{name: "fileExists does not expand tilde", text: `{{ fileExists "~/` + filepath.Base(present) + `" }}`, want: "false"},
		{name: "values", text: `{{ .Values.paths.code }}`, want: "/home/me/code"},
		{name: "readFile strips trailing newlines", text: `{{ readFile "` + present + `" }}`, want: "x"},
		{name: "readFile keeps inner newlines", text: `{{ readFile "` + multiline + `" }}`, want: "one\ntwo"},
		{name: "readFile quoted", text: `{{ readFile "` + present + `" | printf "%q" }}`, want: `"x"`},
		{name: "join layers", text: `{{ join "," .Layers }}`, want: "base,personal,local"},
		{name: "join empty", text: `{{ join "," (slice .Layers 0 0) }}`, want: ""},
		{name: "hasLayer present", text: `{{ hasLayer "personal" }}`, want: "true"},
		{name: "hasLayer absent", text: `{{ hasLayer "server" }}`, want: "false"},
		{name: "shellQuote plain", text: `{{ shellQuote "/home/me/.config" }}`, want: `'/home/me/.config'`},
		{name: "shellQuote empty", text: `{{ shellQuote "" }}`, want: `''`},
		{name: "shellQuote single quote", text: `{{ shellQuote "it's" }}`, want: `'it'\''s'`},
		{name: "shellQuote backslash", text: `{{ shellQuote "a\\b" }}`, want: `'a'\\'b'`},
		{name: "shellQuote space and dollar", text: `{{ shellQuote "a b$c" }}`, want: `'a b$c'`},
		{name: "quote plain", text: `{{ quote "#282828" }}`, want: `"#282828"`},
		{name: "quote empty", text: `{{ quote "" }}`, want: `""`},
		{name: "quote quote and backslash", text: `{{ quote "say \"hi\" \\ now" }}`, want: `"say \"hi\" \\ now"`},
		{name: "quote named escapes", text: `{{ quote "a\b\f\n\r\tb" }}`, want: `"a\b\f\n\r\tb"`},
		{name: "quote other control characters", text: `{{ quote "\x01\x1b\x7f" }}`, want: `"\u0001\u001B\u007F"`},
		{name: "quote characters YAML rejects or folds", text: `{{ quote "a\u0080b\u0085c\u009fd\u2028e\u2029f\ufffeg\uffffh" }}`, want: `"a\u0080b\u0085c\u009Fd\u2028e\u2029f\uFFFEg\uFFFFh"`},
		{name: "quote keeps non-ascii", text: `{{ quote "héllo ❯" }}`, want: `"héllo ❯"`},
		{name: "quote from pipeline", text: `{{ "x" | quote }}`, want: `"x"`},
		{name: "files one pattern", text: `{{ range files "envs/env.{layer}" }}{{ . }};{{ end }}`, want: "/src/envs/env.base;"},
		{name: "files several patterns", text: `{{ join " " (files "a.{layer}" "b.{layer}") }}`, want: "/src/a.base /src/b.base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.name, []byte(tt.text), ctx)

			if err != nil {
				t.Fatalf("Render(%q) error = %v", tt.text, err)
			}
			if string(got) != tt.want {
				t.Errorf("Render(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestRenderFailures(t *testing.T) {
	unreadable := t.TempDir()
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "parse error", text: `{{ if .Home }}unclosed`, want: "unexpected EOF"},
		{name: "unknown field", text: `{{ .Target }}`, want: "can't evaluate field Target"},
		{name: "unknown function", text: `{{ now }}`, want: `function "now" not defined`},
		{name: "missing map key", text: `{{ index .Lookup "nope" }}`, want: "can't evaluate field Lookup"},
		{name: "join with a non-list", text: `{{ join "," .Home }}`, want: "wrong type for value"},
		{name: "readFile missing", text: `{{ readFile "` + filepath.Join(unreadable, "..", "missing") + `" }}`, want: "no such file"},
		{name: "readFile directory", text: `{{ readFile "` + filepath.Dir(unreadable) + `" }}`, want: "is a directory"},
		{name: "fileExists under an unreadable directory", text: `{{ fileExists "` + filepath.Join(unreadable, "x") + `" }}`, want: "permission denied"},
		{name: "quote number", text: `{{ quote 14 }}`, want: "expected string; found 14"},
		{name: "quote invalid utf-8", text: `{{ quote "\xff" }}`, want: "not valid UTF-8"},
		{name: "files without a pattern", text: `{{ files }}`, want: "files: at least one pattern is required"},
		{name: "delims with one word", text: "# overlay:delims <<\n", want: `overlay:delims needs two delimiters on the first line, got "# overlay:delims <<"`},
		{name: "delims with equal words", text: "# overlay:delims ## ##\n", want: `overlay:delims delimiters must differ, got "##" twice`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "fileExists under an unreadable directory" && os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}

			got, err := Render("failing", []byte(tt.text), testContext(t))

			if err == nil {
				t.Fatalf("Render(%q) = %q, nil; want an error containing %q", tt.text, got, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Render(%q) error = %v, want containing %q", tt.text, err, tt.want)
			}
			if !strings.Contains(err.Error(), "failing") {
				t.Errorf("Render(%q) error = %v, want the template name", tt.text, err)
			}
			if got != nil {
				t.Errorf("Render(%q) = %q, want nil on failure", tt.text, got)
			}
		})
	}
}

func TestRenderDelimsDirective(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "replaces the delimiters and drops the line", text: "# overlay:delims << >>\nos=<< .OS >> literal {{ .Nope }}\n", want: "os=linux literal {{ .Nope }}\n"},
		{name: "ignores the comment syntax around it", text: "/* overlay:delims [[ ]] */\n[[ .OS ]]\n", want: "linux\n"},
		{name: "keeps a CRLF body", text: "# overlay:delims << >>\r\n<< .OS >>\r\n", want: "linux\r\n"},
		{name: "directive alone", text: "# overlay:delims << >>", want: ""},
		{name: "second line is not a directive", text: "{{ .OS }}\n# overlay:delims << >>\n", want: "linux\n# overlay:delims << >>\n"},
		{name: "shebang on the second line becomes the first", text: "# overlay:delims << >>\n#!/bin/sh\necho << .OS >>\n", want: "#!/bin/sh\necho linux\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.name, []byte(tt.text), testContext(t))

			if err != nil {
				t.Fatalf("Render(%q) error = %v", tt.text, err)
			}
			if string(got) != tt.want {
				t.Errorf("Render(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestFilesFailsWithoutASelector(t *testing.T) {
	ctx := testContext(t)
	ctx.Files = nil

	_, err := Render("unselectable", []byte(`{{ files "a.{layer}" }}`), ctx)

	if err == nil || !strings.Contains(err.Error(), "files: no source selection configured") {
		t.Errorf("Render() error = %v, want one naming the missing selector", err)
	}
}

func TestFilesPassesPatternsThroughAndNamesSelectorErrors(t *testing.T) {
	var got []string
	ctx := testContext(t)
	ctx.Files = func(patterns []string) ([]string, error) {
		got = patterns
		return nil, errors.New("a.{layer}: layer base matches in two sources: x, y")
	}

	_, err := Render("colliding", []byte(`{{ files "a.{layer}" "b.{layer}" }}`), ctx)

	if diff := cmp.Diff([]string{"a.{layer}", "b.{layer}"}, got); diff != "" {
		t.Errorf("selector patterns mismatch (-want +got):\n%s", diff)
	}
	if err == nil || !strings.Contains(err.Error(), "files: a.{layer}: layer base matches in two sources: x, y") {
		t.Errorf("Render() error = %v, want the selector's error under files", err)
	}
}

func TestMissingMapKeyIsAnError(t *testing.T) {
	// Context has no map fields today; this pins the option so a future map
	// field fails on a typo instead of rendering "<no value>".
	parsed, err := newTemplate("mapped", Context{}).Parse(`{{ .shel }}`)
	if err != nil {
		t.Fatal(err)
	}

	err = parsed.Execute(&strings.Builder{}, map[string]string{"shell": "fish"})

	if err == nil || !strings.Contains(err.Error(), `map has no entry for key "shel"`) {
		t.Errorf("Execute() error = %v, want a missing key error", err)
	}
}
