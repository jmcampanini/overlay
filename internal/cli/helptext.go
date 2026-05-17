package cli

// sourceSelectionHelp documents how source roots are resolved for commands
// that walk overlay source files.
const sourceSelectionHelp = `
Source selection:

  Positional args select source/package roots for this invocation, replacing
  configured sources:

    overlay plan pi codex
    overlay diff pi
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

    1. --profiles a,b,c
    2. OVERLAY_PROFILES=a,b,c
    3. .overlay.toml profiles
    4. default []

  After raw loading, the comma-split value of the env var named by
  env_profiles (if set) is appended. Duplicates are removed, first occurrence
  kept.

Within the effective set, the merge layer order is always:

  base -> each profile in list order -> local

The names "base" and "local" are reserved and cannot appear in any
profile list.`

// diffOutputHelp documents the diff subcommand's output format, exit
// codes, and suggested pipes.
const diffOutputHelp = `
Output format:
  Standard git-style unified diff on stdout. Each group that differs
  produces a block with --- a/<path>, +++ b/<path>, and @@ hunk headers.

Exit codes:
  0   all target files match the rendered output (no drift)
  1   at least one file differs from its rendered output
  2   resolution or I/O error

Suggested pipes for easier reading:
  overlay diff | delta
  overlay diff | bat --language=diff
  overlay diff | git diff --no-index --color /dev/null /dev/stdin
  overlay diff | diff-so-fancy`
