package cmd

// Shared help fragments compose command long descriptions so repeated
// contract text cannot drift between commands. Fragments carry no leading or
// trailing newline; compose them with explicit separators.

// sourceSelectionHelp documents how source roots are resolved for commands
// that walk overlay source files.
const sourceSelectionHelp = `Source selection:

  Positional args select source/package roots for this invocation, replacing
  configured sources:

    overlay plan pi codex
    overlay diff pi
    overlay orphans pi
    overlay render pi codex

  If no positional sources are provided, --source/--sources overrides are used
  (--source adds one value; --sources accepts comma-separated values).
  Otherwise overlay uses .overlay.toml sources, defaulting to ["."]. Relative
  config and positional sources are resolved from the config file's directory
  when a config file exists.

  A configured source directory that does not exist is skipped with a WARN
  line, and a source with no overlay files contributes nothing. Each source
  root is walked on its own and target paths are relative to that root, so a
  package directory named in sources does not appear in the target path.`

// fileConventionHelp documents which source files form a group and how a
// group maps to its target path.
const fileConventionHelp = `File convention:
  A file named <stem>.olay.<profile> or <stem>.olay.<profile>.<ext> is one
  layer of the target <stem>[.<ext>] at the same path relative to its source
  root. The stem may contain dots; <ext> must be one segment, so
  archive.olay.work.tar.gz and settings.olay.work.schema.json are errors.
  With dot_prefix (the default), a directory segment or stem beginning with
  dot- renders with a leading dot: dot-claude/settings.olay.base.json
  renders to .claude/settings.json. A group's active layers are ordered
  base, then each selected profile in list order, then local; base and
  local are reserved names, base is optional, and a group with no active
  layer is skipped with an INFO line. Two groups that render the same
  target path are an error. Directories whose name begins with '.' are
  skipped unless traverse_hidden is set, paths matching ignore are skipped,
  and .gitignore rules apply when respect_gitignore is set. The walk does
  not descend into symlinked directories; a layer file that is a symlink is
  read through the link.`

// mergeSemanticsHelp documents how active layers compose into one target.
const mergeSemanticsHelp = `Merge semantics:
  Targets with a json, toml, yaml, or yml extension (matched
  case-insensitively) merge their active layers in layer order: maps
  deep-merge with override keys replacing base keys, lists of scalars
  concatenate with duplicates dropped in first-seen order, lists holding
  any non-scalar element (objects, nested lists, nulls) concatenate without
  deduplication, and any other pair (scalars, mismatched types) takes the
  override. Output keys are alphabetized and JSON and YAML numbers are
  normalized (1.0 may render as 1), so output is deterministic and diffs
  stay stable; source key order, comments, and formatting are not kept.
  YAML layers must be single-document root mappings (an empty or
  comment-only layer counts as an empty map); a root sequence or scalar, an
  explicit root null, a multi-document stream, a non-string or complex key,
  an alias, or a custom tag is an error. Rendered YAML is block style with
  2-space indentation; TOML tables are unindented unless toml_indent_tables
  is set. The exact yaml or yml spelling, including case, is part of the
  group identity and is kept in the target name, and every YAML layer of
  one stem in one directory must use the same spelling, inactive profiles
  included: config.olay.base.yaml beside config.olay.work.yml is an error.

  Every other extension, and no extension, copies the last active layer as
  a whole file. A [[render_rules]] entry whose path equals the
  target-relative path (after dot_prefix mapping, with exact extension
  spelling) forces 'append', which concatenates the active layers in layer
  order inserting one newline between non-empty layers when the previous
  one does not already end with one, or 'copy', which copies the last
  active layer even for a mergeable extension.`

// profilePrecedenceHelp documents how the active profile set is resolved.
// It is attached to every subcommand that consumes profiles so the rules
// are always visible in --help output.
const profilePrecedenceHelp = `Profile resolution:

  Raw profiles are loaded with normal config precedence:

    1. --profiles a,b,c or repeated --profile NAME flags
    2. OVERLAY_PROFILES=a,b,c
    3. .overlay.toml profiles
    4. default []

  --profile is pflag-only; there is no profile TOML key or OVERLAY_PROFILE.
  If both CLI forms are used, --profiles values are applied first, then
  repeated --profile values. After raw loading, the comma-split values of
  each env var listed in env_profiles are appended, in list order; unset or
  empty-valued vars are skipped. Duplicates are removed, first occurrence
  kept.

Within the effective set, the layer order is always:

  base -> each profile in list order -> local

The names "base" and "local" are reserved and cannot appear in any
profile list. Any other name is a valid profile. An env_profiles entry that
is blank or padded with whitespace is an error.`

// varsPrecedenceHelp documents how variable pins are resolved. It is
// attached to every subcommand that substitutes variables so the rules are
// always visible in --help output.
const varsPrecedenceHelp = `Variable substitution:

  When .overlay.toml sets substitute_prefixes, ${NAME} references whose names
  match a listed prefix are replaced. Mergeable JSON/TOML/YAML layers
  substitute string values and mapping keys before merge; copy and append
  targets substitute final bytes. Values resolve with this precedence, highest
  to lowest:

    1. --vars A=1,B=2 then repeated --var NAME=value (later wins per name)
    2. OVERLAY_VARS=A=1,B=2 (fully replaced when any vars flag is set)
    3. ambient process environment

  --vars and OVERLAY_VARS are comma-split; values containing commas must use
  --var. An exact duplicate NAME=value collapses to its first position, so
  re-pinning a value passed earlier will not override a different value given
  in between. Pins whose names match no configured prefix are errors. A
  reference to an unset variable fails the run before anything is written;
  write $${NAME} to emit a literal ${NAME}. See 'overlay docs' for the full
  reference.

  With substitute_prefixes unset, substitution is off and ${...} text renders
  unchanged. There is no vars key in .overlay.toml. A reference whose name
  matches no listed prefix, such as ${HOME}, passes through unchanged, and a
  substituted value is never re-scanned. A target whose target-relative path
  matches a substitute_exclude pattern is not substituted, escapes included;
  as with ignore, a pattern with no '/' matches by base name at any depth.
  The failure for unset variables names every failing target and all of its
  missing variables; a variable set to the empty string substitutes as
  empty. In mergeable targets a missing variable in any active layer fails
  the target, even when a later layer overrides that value, and key
  substitution that creates duplicate keys within one map in one layer is
  an error. A pinned variable that no target consumes logs a WARN line.`

// renderStateHelp documents the ownership registry that render maintains.
const renderStateHelp = `Ownership state:
  By default, render records successfully written targets in
  .overlay.state.json for later orphan detection. --no-state writes targets
  normally without reading, validating, creating, garbage-collecting, or
  updating that file. Targets that alias the state path remain rejected.
  The file lives beside the loaded config file, or in the current directory
  when no config file is loaded. It is rewritten atomically at the end of
  every render that maintains it, failed renders included, and keeps earlier
  entries whose targets still exist as regular files. It records absolute
  paths for this machine, so add it to .gitignore rather than committing it.
  A malformed state file fails the render before anything is written; with
  --no-state it is left untouched and does not block the render.`

// streamContractHelp documents stdout and stderr roles for commands that
// resolve configuration and log through the shared logger.
const streamContractHelp = `Streams:
  Payload goes to stdout. Log lines go to stderr prefixed INFO, WARN, ERRO,
  or DEBU and are colored only when stderr is a terminal; --quiet drops INFO
  lines, --verbose adds DEBU lines, and --verbose wins when both are set. A
  failure that ends the run is reported on stderr, as an ERRO line or as a
  final 'overlay: <message>' line, never on stdout. Nothing prompts or reads
  stdin.`

// configPrecedenceHelp documents where configuration is discovered and how
// the layers combine. It is shared by the root and config commands.
const configPrecedenceHelp = `Configuration precedence:
  Settings load in this order, and a later layer replaces any value an
  earlier one sets: built-in defaults; .overlay.toml in the current
  directory when it exists, or the file named by --config, which must exist
  (parent directories and user-level locations are never searched); the
  environment variables OVERLAY_SOURCES, OVERLAY_TARGET, OVERLAY_PROFILES,
  OVERLAY_CONTINUE, and OVERLAY_VARS; then the flags --sources/--source,
  --target, --profiles/--profile, --continue, and --vars/--var, which every
  command accepts (docs ignores them). dot_prefix, env_profiles,
  toml_indent_tables, ignore, traverse_hidden, respect_gitignore,
  render_rules, substitute_prefixes, and substitute_exclude are read only
  from the file; an OVERLAY_ variable for one of them is ignored. An unknown
  key in the file, including vars, is an error. target must be set by the
  file, the environment, or --target for every command except docs and the
  config report.`

// diffOutputHelp documents the diff subcommand's output format, exit
// codes, and suggested pipes.
const diffOutputHelp = `Output format:
  Standard git-style unified diff on stdout. Each group that differs
  produces a block with --- a/<path>, +++ b/<path>, and @@ hunk headers.

Exit codes:
  0   all target files match the rendered output (no drift)
  1   at least one file differs from its rendered output
  2   resolution, render (e.g. missing variables), or I/O error

Suggested pipes for easier reading:
  overlay diff | delta
  overlay diff | bat --language=diff
  overlay diff | git diff --no-index --color /dev/null /dev/stdin
  overlay diff | diff-so-fancy`

// orphansOutputHelp documents the orphans subcommand's output formats and
// exit codes.
const orphansOutputHelp = `Output format:
  By default, absolute target paths on stdout, one per line.
  With --json, a top-level JSON array of those paths in the same order.
  Successful JSON output always ends with a newline and is [] when empty.
  Paths are JSON-escaped (newlines, quotes, backslashes); &, <, and > stay
  literal. Neither form carries terminal escapes, and stdout stays empty
  when detection fails.

Exit codes:
  0   no orphaned targets found
  1   at least one orphaned target found
  2   resolution, state, discovery, path, or I/O error`
