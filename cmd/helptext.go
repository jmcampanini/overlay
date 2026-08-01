package cmd

// sourceSelectionHelp documents how source roots are resolved for commands
// that walk overlay source files.
const sourceSelectionHelp = `
Source selection:

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
  when a config file exists.`

// profilePrecedenceHelp documents how the active profile set is resolved.
// It is attached to every subcommand that consumes profiles so the rules
// are always visible in --help output.
const profilePrecedenceHelp = `
Profile resolution:

  Raw profiles are loaded with normal config precedence:

    1. --profiles a,b,c or repeated --profile NAME flags
    2. OVERLAY_PROFILES=a,b,c
    3. .overlay.toml profiles
    4. default []

  --profile is pflag-only; there is no profile TOML key or OVERLAY_PROFILE.
  If both CLI forms are used, --profiles values are applied first, then repeated
  --profile values. After raw loading, the comma-split values of each env var
  listed in env_profiles are appended, in list order; unset or empty-valued
  vars are skipped. Duplicates are removed, first occurrence kept.

Within the effective set, the layer order is always:

  base -> each profile in list order -> local

The names "base" and "local" are reserved and cannot appear in any
profile list.`

// varsPrecedenceHelp documents how variable pins are resolved. It is
// attached to every subcommand that substitutes variables so the rules are
// always visible in --help output.
const varsPrecedenceHelp = `
Variable substitution:

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
  reference.`

const renderStateHelp = `
Ownership state:
  By default, render records successfully written targets in
  .overlay.state.json for later orphan detection. --no-state writes targets
  normally without reading, validating, creating, garbage-collecting, or
  updating that file. Targets that alias the state path remain rejected.`

// diffOutputHelp documents the diff subcommand's output format, exit
// codes, and suggested pipes.
const diffOutputHelp = `
Output format:
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

const orphansOutputHelp = `
Output format:
  By default, absolute target paths on stdout, one per line.
  With --json, a top-level JSON array of those paths in the same order.
  Successful JSON output always ends with a newline and is [] when empty.

Exit codes:
  0   no orphaned targets found
  1   at least one orphaned target found
  2   resolution, state, discovery, path, or I/O error`
