package config

// SchemaDocs is the canonical reference for the .overlay.toml file format.
// It is printed by the `overlay docs` subcommand.
const SchemaDocs = `overlay configuration reference (.overlay.toml)

The .overlay.toml file lives in the directory you run 'overlay' from, or at
the path passed via --config. All fields are optional except 'target', which
must be set by the file, OVERLAY_TARGET, or --target for runtime commands.

FIELDS

  sources = ["."]
    type:    array of strings
    default: ["."]
    The source directories to walk when searching for *.olay.*.* files. Each
    source is treated as its own root: target paths are rendered relative to
    that source directory. TOML relative paths are resolved from the config
    file's directory at runtime. Legacy files may use source = "dir" for a
    single source; new configs should use sources, and a file may not set both.

  target = "~/"
    type:    string
    default: (none — required)
    The directory where merged files are written. A leading "~" is expanded
    to the current user's home directory; "$VAR" and "${VAR}" are expanded
    from the environment. TOML relative paths are resolved from the config
    file's directory at runtime. Overlay exits with an error if target is
    empty after config + environment + flag loading.

  dot_prefix = true
    type:    boolean
    default: true
    When true, any path segment beginning with "dot-" is rewritten with a
    leading "." (e.g. "dot-claude" -> ".claude") in the output path. This
    is a convenience for dotfiles layouts.

  profiles = ["work"]                  # example
    type:    array of strings
    default: []
    Raw profiles to activate, in merge order. This value can be overridden
    by OVERLAY_PROFILES or --profiles. After raw loading, env_profiles may
    append more profiles. The names "base" and "local" are reserved and
    cannot appear in the effective list.

  env_profiles = "DOTFILES_PROFILE"    # example
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
    groups, exiting non-zero at the end if any failed. OVERLAY_CONTINUE and
    the --continue CLI flag are equivalent config-backed overrides.

  ignore = []
    type:    array of strings (doublestar glob patterns)
    default: []
    Paths matching any of these patterns are skipped during each source
    walk. Patterns support ** (match any number of path segments). Hidden
    directories are already skipped by default via traverse_hidden, so ".git"
    does not need to be listed unless you enable that.

  traverse_hidden = false
    type:    boolean
    default: false
    When false (default), directories whose name begins with "." are
    skipped during each source walk. Set to true to descend into them.

  respect_gitignore = false
    type:    boolean
    default: false
    When true, overlay respects .gitignore rules while walking each source
    directory. This is off by default to keep walking cheap and predictable.

CONFIG-BACKED ENVIRONMENT VARIABLES

  OVERLAY_SOURCES    overrides sources (comma-separated)
  OVERLAY_TARGET     overrides target
  OVERLAY_PROFILES   overrides raw profiles (comma-separated)
  OVERLAY_CONTINUE   overrides continue_on_error

SOURCE RESOLUTION PRECEDENCE

Source roots are loaded from these sources, highest to lowest:

  1. positional command args for plan, diff, and render (e.g. overlay plan pi)
  2. --source / --sources CLI flags
  3. OVERLAY_SOURCES env var
  4. .overlay.toml sources
  5. default ["."]

Positional sources are resolved relative to the config file directory when a
config file exists. This supports stow-style package selection:

  overlay --config ~/dotfiles/.overlay.toml plan pi codex

PROFILE RESOLUTION PRECEDENCE

Raw profiles are loaded from these sources, highest to lowest:

  1. --profiles CLI flag
  2. OVERLAY_PROFILES env var
  3. .overlay.toml profiles
  4. default []

After raw loading, Overlay appends the comma-split value of the env var named
by env_profiles (if set). Duplicates are removed, preserving first occurrence.

Within the effective set, the merge layer order is always:
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

With a stow-style config, the package directory is the source root and is not
part of the target path:

  sources = ["pi"]
  pi/dot-pi/agent/models.olay.base.json -> ~/.pi/agent/models.json

See README.md for a worked example.
`
