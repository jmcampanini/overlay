package cli

// profilePrecedenceHelp documents how the active profile set is resolved.
// It is attached to every subcommand that consumes profiles so the rules
// are always visible in --help output.
const profilePrecedenceHelp = `
Profile resolution (highest precedence first):

  1. --profiles a,b,c           CLI flag. Replaces the entire set.
                                Order on the command line is preserved.

  2. .overlay.toml              config.profiles (in listed order), then
                                the comma-split value of the env var named
                                by config.env_profiles (if set), appended.
                                Duplicates removed, first occurrence kept.

  3. OVERLAY_PROFILES env var   Used only when no .overlay.toml exists and
                                no --profiles flag was given.

Within the resolved set, the merge layer order is always:

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
