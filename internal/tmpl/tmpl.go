// Package tmpl renders overlay templates: the context fields and functions
// here are overlay's public API for .tmpl files, so every change to them is a
// change to what users can write.
package tmpl

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"text/template"
)

// Context is the data every template executes against.
type Context struct {
	// Home is the user's home directory, whatever the render target is.
	Home string
	// Hostname is the machine's hostname exactly as the kernel reports it.
	Hostname string
	// Layers is the complete layer order, including base and local.
	Layers []string
	// OS is the Go runtime's operating system name, such as darwin or linux.
	OS string
	// Sources lists every configured source in configuration order.
	Sources []Source
}

// Source is one configured source as a template sees it.
type Source struct {
	Name string
	// Path is the absolute checkout directory.
	Path string
}

// Render parses text as a text/template named name and executes it against
// ctx. A missing map key, a missing field, a parse error, or a function
// error fails the whole render; nothing is partially returned.
func Render(name string, text []byte, ctx Context) ([]byte, error) {
	parsed, err := newTemplate(name).Parse(string(text))
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := parsed.Execute(&out, ctx); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// newTemplate returns an empty template with overlay's options and
// functions: a missing map key is an error, and fileExists, join, and
// shellQuote are available beyond text/template's builtins.
func newTemplate(name string) *template.Template {
	return template.New(name).Option("missingkey=error").Funcs(template.FuncMap{
		"fileExists": fileExists,
		"join":       join,
		"shellQuote": shellQuote,
	})
}

// fileExists reports whether anything exists at path: a file, directory, or
// other entry, following symbolic links. The path is used as given, so a
// relative path resolves against the working directory and ~ is not
// expanded. An error other than absence, such as permission denied on a
// parent directory, fails the template.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("fileExists %q: %w", path, err)
}

// join concatenates items with sep between them, mirroring strings.Join
// with the separator first so templates read as join "," .Layers.
func join(sep string, items []string) string {
	return strings.Join(items, sep)
}

// shellQuote quotes s so bash, zsh, and fish all read it as one literal
// word. The result is single-quoted; for a quote or a backslash inside s it
// closes the quotes, writes a backslash and the character, and reopens them,
// because fish, unlike POSIX shells, interprets backslashes inside single
// quotes.
func shellQuote(s string) string {
	var out strings.Builder
	out.WriteByte('\'')
	for _, r := range s {
		if r == '\'' || r == '\\' {
			out.WriteString(`'\`)
			out.WriteRune(r)
			out.WriteByte('\'')
			continue
		}
		out.WriteRune(r)
	}
	out.WriteByte('\'')
	return out.String()
}
