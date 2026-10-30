package cli

import "github.com/spf13/cobra"

func templatesTopic() *cobra.Command {
	return &cobra.Command{
		Use:   "templates",
		Short: "Template data, functions, delimiters, and idioms",
		Long: `A file whose name ends in .tmpl is a Go text/template and renders to the
target path without that suffix, so envs/dot-shellenv.tmpl becomes
~/.shellenv with the template's permission bits. A fragment named
NAME.olay.LAYER.EXT.tmpl or NAME.olay.LAYER.tmpl is a template of a
fragment, and its output composes like any other fragment; 'overlay
render --help' describes fragments and when templates run. A template
that fails to parse or execute fails the whole render before anything is
written, and a missing field or map key is an error rather than empty
output.

Every template sees the same data:

  .Layers    the layer order as a list, including base and local
  .Sources   the configured sources in order; each has .Name and .Path,
             the absolute checkout directory
  .Home      the home directory, whatever --target is
  .OS        the operating system as Go names it: darwin or linux
  .Hostname  the hostname exactly as the system reports it
  .Values    the merged values document 'overlay render --help'
             describes, so a file holding [theme] followed by
             bg = "#282828" renders {{ .Values.theme.bg }}; a key no file
             set is an error, and an optional one is guarded with
             {{ with index .Values "theme" }} ... {{ end }}

Beyond the text/template builtins, templates may call:

  env NAME         the environment variable's value; an unset variable
                   fails the render, and a variable set to "" is set
  envOr NAME DEF   the environment variable's value, or DEF when unset
  fileExists PATH  true when anything exists at PATH (a file, directory,
                   or other entry, following symbolic links); PATH is used
                   as given, ~ is not expanded, and an error other than
                   absence fails the render
  files PATTERN... the absolute paths 'overlay files PATTERN...' prints
                   for the same patterns and layers, as a list: each
                   PATTERN is relative to every source root and holds
                   {layer}, matches order by layer, then source, then
                   pattern, a layer a source's manifest does not declare
                   never matches there, and no match is an empty list;
                   a malformed pattern or one pattern matching in two
                   sources for one layer fails the render
  hasLayer NAME    true when NAME is one of the layers in .Layers
  join SEP LIST    LIST's items concatenated with SEP between them
  quote S          S in double quotes using only the escapes JSON, TOML,
                   and YAML all accept: \" and \\, \b \f \n \r \t, and
                   \u00XX for any other control character; S must be a
                   string, and any other value fails the render
  readFile PATH    the file's content with trailing newlines removed, as
                   $(cat PATH) would give; PATH is used as given, and a
                   missing or unreadable file fails the render, so guard
                   an optional file with fileExists
  shellQuote S     S single-quoted so bash, zsh, and fish read one literal
                   word; quotes and backslashes inside S are escaped
                   outside the quotes

Prefer quote to printf "%q": %q writes Go escapes such as \x1b and \a,
which TOML and JSON reject, and turns a number into a rune literal.

A template whose first line holds overlay:delims LEFT RIGHT uses LEFT and
RIGHT as its action delimiters instead of {{ and }}, so a file whose own
syntax uses {{ needs no escaping. The directive may sit inside the file's
comment syntax: the first two words after overlay:delims are the
delimiters, and the rest of the line, including a comment closer, is
ignored. The whole line is dropped from the output. Fewer than two words,
or two equal words, fail the render. Only the first line is read, so a
templated script puts its #! line second, which becomes the first line
of the output. A plain file is copied as it is, so a directive in one
stays in the output. For example, this fragment keeps Command K's own
{{sq .path}} expressions and still reads a theme value:

  # overlay:delims << >>
  [[actions]]
  cmd = "idea {{sq .path}}"

  [theme.dark]
  accent = "<< .Values.theme.colors.accent >>"

Quotes can stay in the file. accent = "{{ .Values.theme.colors.accent }}"
renders a valid TOML, JSON, or YAML string whenever the value holds no
quote, backslash, or control character, which is every color and path;
use quote for a value that might. with removes a repeated prefix, a
variable names a value used more than once, and {{- or -}} trims the
newline beside an action that produces no output:

  {{- with .Values.theme.colors }}
  accent = "{{ .accent }}"
  text = "{{ .fg }}"
  {{- end }}

  {{- $code := env "CODE_DIR" }}
  path = "{{ $code }}/github.com/jmcampanini/dotfiles-base"
  launch_path = "{{ $code }}/github.com/jmcampanini/dotfiles-base"

A secret can stay in a file outside every source and be read at render
time, for example {{ readFile (printf "%s/.config/overlay/local/api.key"
.Home) }}. The plan output never prints rendered content, and the target
takes the template's permission bits, so keep such a template at 600.

This loader sources every envs/env.LAYER a source declares a layer for,
in layer order and then source order, and renders nothing for a layer no
source has a file for:

  {{ range files "envs/env.{layer}" -}}
  source {{ shellQuote . }}
  {{ end }}

'overlay help templates' prints this text on stdout, prompts for nothing,
and needs no terminal.`,
		GroupID: helpTopicsGroupID,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
