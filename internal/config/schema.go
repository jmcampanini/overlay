package config

// SchemaDocs is the canonical reference for the .overlay.toml file format.
// It is printed by the `overlay docs` subcommand and used as the source of
// truth for per-field comments in `overlay config` output.
const SchemaDocs = `overlay configuration reference (.overlay.toml)

The .overlay.toml file lives in the directory you run 'overlay' from, or at
the path passed via --config. All fields are optional except 'target', which
must be set either in the file or via the --target flag.

FIELDS

  source = "."
    type:    string
    default: "."
    The directory to walk when searching for *.olay.*.* files. Relative
    paths are resolved from the current working directory.

  target = "~/"
    type:    string
    default: (none — required)
    The directory where merged files are written. A leading "~" is expanded
    to the current user's home directory; "$VAR" and "${VAR}" are expanded
    from the environment. Overlay exits with an error if target is empty
    after config + flag resolution.

  dot_prefix = true
    type:    boolean
    default: true
    When true, any path segment beginning with "dot-" is rewritten with a
    leading "." (e.g. "dot-claude" -> ".claude") in the output path. This
    is a convenience for dotfiles layouts.

  profiles = ["work"]
    type:    array of strings
    default: []
    Profiles to activate, in merge order. The layers are applied as
    base -> <profile_1> -> <profile_2> -> ... -> local. The names "base"
    and "local" are reserved and cannot appear here.

  env_profiles = "DOTFILES_PROFILE"
    type:    string
    default: ""
    Optional environment variable name. If set, overlay reads that env var
    at runtime and APPENDS its comma-separated value to the 'profiles' list
    (with duplicates removed, preserving first occurrence).

  continue_on_error = false
    type:    boolean
    default: false
    When false (default), overlay fails fast on the first invalid source
    file. When true, it logs the error and continues with the remaining
    groups, exiting non-zero at the end if any failed. The --continue
    CLI flag is equivalent.

  ignore = []
    type:    array of strings (doublestar glob patterns)
    default: []
    Paths matching any of these patterns are skipped during the source
    walk. Patterns support ** (match any number of path segments).
    Hidden directories are already skipped by default via traverse_hidden,
    so ".git" does not need to be listed unless you enable that.

  traverse_hidden = false
    type:    boolean
    default: false
    When false (default), directories whose name begins with "." are
    skipped during the source walk. Set to true to descend into them.

  respect_gitignore = false
    type:    boolean
    default: false
    When true, overlay respects .gitignore rules while walking the source
    directory. This is off by default to keep walking cheap and predictable.

PROFILE RESOLUTION PRECEDENCE

The active profile set is resolved from three sources, highest to lowest:

  1. --profiles CLI flag  (replaces the entire set, order preserved)
  2. .overlay.toml        (profiles list + appended env_profiles env var)
  3. OVERLAY_PROFILES     (env var, only when no .overlay.toml is found
                           and no --profiles flag was given)

Within the resolved set, the merge layer order is always:
  base (if present) -> each profile in list order -> local (if present)

FILE CONVENTION

Overlay discovers files by the pattern <stem>.olay.<profile>.<ext> where
<ext> is "json" or "toml". The profile name "base" merges first, "local"
merges last, and any other name is a user profile. For example:

  dot-claude/settings.olay.base.json      -> base layer (always first)
  dot-claude/settings.olay.work.json      -> "work" profile layer
  dot-claude/settings.olay.local.json     -> local layer (always last)

With dot_prefix = true and target = "~/", those layers merge into:

  ~/.claude/settings.json

See README.md for a worked example.
`
