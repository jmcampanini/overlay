package tmpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testContext(t *testing.T) Context {
	t.Helper()
	return Context{
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
	t.Setenv("OVERLAY_TEST_SECRET", "hunter2")
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
		{name: "env set", text: `{{ env "OVERLAY_TEST_SECRET" }}`, want: "hunter2"},
		{name: "env unset is empty", text: `{{ if env "OVERLAY_TEST_UNSET" }}set{{ else }}unset{{ end }}`, want: "unset"},
		{name: "readFile strips trailing newlines", text: `{{ readFile "` + present + `" }}`, want: "x"},
		{name: "readFile keeps inner newlines", text: `{{ readFile "` + multiline + `" }}`, want: "one\ntwo"},
		{name: "readFile quoted", text: `{{ readFile "` + present + `" | printf "%q" }}`, want: `"x"`},
		{name: "join layers", text: `{{ join "," .Layers }}`, want: "base,personal,local"},
		{name: "join empty", text: `{{ join "," .Layers }}`, want: ""},
		{name: "shellQuote plain", text: `{{ shellQuote "/home/me/.config" }}`, want: `'/home/me/.config'`},
		{name: "shellQuote empty", text: `{{ shellQuote "" }}`, want: `''`},
		{name: "shellQuote single quote", text: `{{ shellQuote "it's" }}`, want: `'it'\''s'`},
		{name: "shellQuote backslash", text: `{{ shellQuote "a\\b" }}`, want: `'a'\\'b'`},
		{name: "shellQuote space and dollar", text: `{{ shellQuote "a b$c" }}`, want: `'a b$c'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext(t)
			if tt.name == "join empty" {
				ctx.Layers = nil
			}

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

func TestMissingMapKeyIsAnError(t *testing.T) {
	// Context has no map fields today; this pins the option so a future map
	// field fails on a typo instead of rendering "<no value>".
	parsed, err := newTemplate("mapped").Parse(`{{ .shel }}`)
	if err != nil {
		t.Fatal(err)
	}

	err = parsed.Execute(&strings.Builder{}, map[string]string{"shell": "fish"})

	if err == nil || !strings.Contains(err.Error(), `map has no entry for key "shel"`) {
		t.Errorf("Execute() error = %v, want a missing key error", err)
	}
}
