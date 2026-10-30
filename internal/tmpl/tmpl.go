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
	"slices"
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
	// Values is the per-machine values document; a key it lacks is an error
	// under missingkey=error.
	Values map[string]any
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
	parsed, err := newTemplate(name, ctx.Layers).Parse(string(text))
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
// functions: a missing map key is an error, and env, envOr, fileExists,
// hasLayer, join, readFile, and shellQuote are available beyond
// text/template's builtins. hasLayer reports whether its argument is one of
// layers.
func newTemplate(name string, layers []string) *template.Template {
	return template.New(name).Option("missingkey=error").Funcs(template.FuncMap{
		"env":        env,
		"envOr":      envOr,
		"fileExists": fileExists,
		"hasLayer":   func(layer string) bool { return slices.Contains(layers, layer) },
		"join":       join,
		"readFile":   readFile,
		"shellQuote": shellQuote,
	})
}

// env returns the environment variable name's value. An unset variable fails
// the template, so a secret or path never silently renders empty; a variable
// set to the empty string is set.
func env(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("env %q: variable is not set", name)
	}
	return value, nil
}

// envOr returns the environment variable name's value, or fallback when the
// variable is unset. A variable set to the empty string returns the empty
// string.
func envOr(name, fallback string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	return value
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

// readFile returns the content of the file at path with trailing newlines
// removed, as $(cat path) would in a shell. The path is used as given, like
// fileExists. A missing or unreadable file fails the template; guard an
// optional file with fileExists.
func readFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(content), "\r\n"), nil
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
