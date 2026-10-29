package cli

// Shared help fragments compose command long descriptions so repeated
// contract text cannot drift between commands. Fragments carry no leading or
// trailing newline; compose them with explicit separators.

const configFileHelp = `The configuration file is $XDG_CONFIG_HOME/overlay/config.toml, or
~/.config/overlay/config.toml when XDG_CONFIG_HOME is unset, empty, or
relative. No other location is searched, and a missing discovered file
counts as empty.`

const layerSelectionHelp = `Layers come from the highest layer that sets them: --layers LIST
(comma-separated) and --layer NAME, which may repeat and combine with the
--layers values first, override OVERLAY_LAYERS, which overrides layers in
the file. --layers "" clears this initial list. The file's env_layers names
environment variables whose comma-separated values append to that list,
in the order named, even when flags set the initial list. Unset variables
and blank entries in their values contribute nothing; surrounding spaces
are trimmed. When a name repeats, its last position wins, so
'work,laptop,work' becomes 'laptop,work'. Names are lowercase letters or
digits in groups separated by single hyphens, at most 64 characters;
blank entries in the initial list are errors. 'base' and 'local' are
automatic: base applies before the configured layers and local after,
and neither may be listed.`

const contractHelp = `Diagnostics go to stderr, and a failure is one line, 'overlay: <message>',
with the exit status 'overlay help exit-codes' describes. overlay itself
never prompts, and nothing works only interactively.`

const fileSelectionHelp = `Each pattern selects files relative to the root of each configured source
checkout. Patterns must be nonempty and contain {layer} at least once;
every occurrence is replaced. They must not start with / or ~, contain a
.. path segment, contain braces outside {layer}, or contain *, ?, or [
(reserved for globs). Quote patterns to keep the shell from expanding them.

Matches are ordered by layer (base, effective layers, local), then source
in configuration order, then pattern in operand order. A regular file or
a symbolic link to one matches; missing paths, directories, and other
entries contribute nothing. Nothing is modified.

Before matching, every source's overlay.toml is read and validated as
'overlay render --help' describes; a missing or invalid manifest fails. A
source matches only for layers its manifest declares with a [layers.NAME]
table, base and local included, even when the table lists no packages.
Packages do not limit matching: patterns may select files anywhere in the
source.

A stat error other than absence fails. One pattern matching in two sources
for the same layer fails, naming the pattern, layer, and sources. Different
patterns or layers never conflict. Files are not deduplicated, so multiple
patterns may select the same file more than once. All matches and
collisions are checked before stdout is written. No matches means empty
stdout and exit 0. Successful commands write nothing to stderr.`
