// Package tmpl renders overlay templates: the context fields, functions,
// and directive here are overlay's public API for .tmpl files, so every
// change to them is a change to what users can write.
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
	"unicode/utf8"
)

// delimsDirective marks a first line that names the action delimiters a
// template uses instead of {{ and }}.
const delimsDirective = "overlay:delims"

// Context is the data every template executes against.
type Context struct {
	// Files selects source files for the files function: it receives the
	// patterns as given and returns absolute paths in layer, source, and
	// pattern order. Templates call files rather than this field; a nil
	// Files makes files fail.
	Files func(patterns []string) ([]string, error)
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
// ctx. When the first line of text holds the overlay:delims directive, the
// two words after it are the action delimiters and the line is dropped
// from the output. A malformed directive, a missing map key, a missing
// field, a parse error, or a function error fails the whole render;
// nothing is partially returned.
func Render(name string, text []byte, ctx Context) ([]byte, error) {
	body, left, right, err := splitDelimsDirective(text)
	if err != nil {
		return nil, fmt.Errorf("template: %s: %w", name, err)
	}
	parsed, err := newTemplate(name, ctx).Delims(left, right).Parse(string(body))
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := parsed.Execute(&out, ctx); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// splitDelimsDirective returns text without its first line and the two
// delimiters that line names after overlay:delims, or text unchanged and
// empty delimiters, which text/template reads as the defaults, when the
// first line holds no directive. The words before the marker, so the host
// file's comment syntax, and the words after the two delimiters are
// ignored. Fewer than two words after the marker or two equal words are
// errors.
func splitDelimsDirective(text []byte) (body []byte, left, right string, err error) {
	firstLine, rest, _ := bytes.Cut(text, []byte("\n"))
	_, after, found := strings.Cut(string(firstLine), delimsDirective)
	if !found {
		return text, "", "", nil
	}

	words := strings.Fields(after)
	if len(words) < 2 {
		return nil, "", "", fmt.Errorf("%s needs two delimiters on the first line, got %q", delimsDirective, strings.TrimSpace(string(firstLine)))
	}
	if words[0] == words[1] {
		return nil, "", "", fmt.Errorf("%s delimiters must differ, got %q twice", delimsDirective, words[0])
	}
	return rest, words[0], words[1], nil
}

// newTemplate returns an empty template with overlay's options and
// functions: a missing map key is an error, and env, envOr, fileExists,
// files, hasLayer, join, quote, readFile, and shellQuote are available
// beyond text/template's builtins. hasLayer reports whether its argument
// is one of ctx.Layers, and files selects through ctx.Files.
func newTemplate(name string, ctx Context) *template.Template {
	return template.New(name).Option("missingkey=error").Funcs(template.FuncMap{
		"env":        env,
		"envOr":      envOr,
		"fileExists": fileExists,
		"files":      func(patterns ...string) ([]string, error) { return files(ctx.Files, patterns) },
		"hasLayer":   func(layer string) bool { return slices.Contains(ctx.Layers, layer) },
		"join":       join,
		"quote":      quote,
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

// files selects source files through selector, which carries the same
// rules as the files command. No pattern at all or no selector fails the
// template; no match is an empty list.
func files(selector func([]string) ([]string, error), patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, errors.New("files: at least one pattern is required")
	}
	if selector == nil {
		return nil, errors.New("files: no source selection configured")
	}
	paths, err := selector(patterns)
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	return paths, nil
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

// quote returns s in double quotes using only the escapes JSON strings,
// TOML basic strings, and YAML double-quoted scalars all accept: \" and
// \\, \b \f \n \r \t for those characters, and \uXXXX for every character
// YAML does not allow or does not preserve in a quoted scalar. Other
// characters, including non-ASCII ones, are written as they are. Invalid
// UTF-8 fails the template without echoing s, which may be a secret, and
// text/template rejects a non-string argument before quote runs.
func quote(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("quote: the value is not valid UTF-8")
	}

	var out strings.Builder
	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if needsUnicodeEscape(r) {
				fmt.Fprintf(&out, `\u%04X`, r)
				continue
			}
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String(), nil
}

// needsUnicodeEscape reports whether quote must write r as \uXXXX: the C0
// and C1 control characters and DEL, which YAML rejects inside a quoted
// scalar except for NEL, the line breaks NEL, U+2028, and U+2029, which
// YAML folds into a space, and U+FFFE and U+FFFF, which YAML rejects. JSON
// and TOML accept every one of them raw or escaped, so the escape is safe
// in all three formats.
func needsUnicodeEscape(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || r == 0xfffe || r == 0xffff
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
