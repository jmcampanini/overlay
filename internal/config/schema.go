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
    The source directories to walk when searching for <stem>.olay.<profile>
    and <stem>.olay.<profile>.<ext> files. Each source is treated as its own
    root: target paths are rendered relative to that source directory. TOML
    relative paths are resolved from the config
    file's directory at runtime. Missing source directories are skipped with a
    warning; existing directories with no overlay files are no-ops.

  target = "~/"
    type:    string
    default: (none — required)
    The directory where rendered files are written. A leading "~" is expanded
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
    Raw profiles to activate, in layer order. This value can be overridden
    by OVERLAY_PROFILES, --profiles, or repeated --profile NAME flags. The
    --profile flag is pflag-only; there is no singular TOML key or environment
    variable. After raw loading, env_profiles may append more profiles. The
    names "base" and "local" are reserved and cannot appear in the effective
    list.

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

  toml_indent_tables = false
    type:    boolean
    default: false
    When false (default), TOML tables and array-table values are emitted
    without nested indentation. When true, Overlay passes true to the TOML
    encoder's SetIndentTables option, preserving the older indented style.

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

  [[render_rules]]
    type:    repeated table
    default: []
    Optional per-target rules that choose an explicit render strategy. A rule
    matches the rendered target-relative path after normal target mapping,
    including dot_prefix behavior. For example, with dot_prefix = true:

      npm/dot-npmrc.olay.base -> <target>/.npmrc

    is matched by:

      [[render_rules]]
      path = ".npmrc"
      strategy = "append"

    Fields:

      path = ".npmrc"
        Required. Exact rendered target-relative path. Use slash-separated paths
        such as ".ssh/config" or ".claude/settings.json". The source-relative
        overlay name, such as "dot-npmrc", is not the matching API.

      strategy = "append"
        Required. Supported values are exactly:
          append  append active layers in layer order
          copy    copy the highest-precedence active layer

    Without a matching render rule, defaults are unchanged:
      .json/.toml/.yaml/.yml -> merge
      other files -> copy

    Append preserves each active layer's content and appends in normal layer
    order: base, then active profiles, then local. It inserts one newline
    between adjacent non-empty layers only when the previous layer does not
    already end with a newline. It does not trim, de-duplicate, parse syntax, or
    force a final newline.

    Validation rejects missing or empty paths, absolute paths, paths containing
    "..", missing strategies, unsupported strategies, and duplicate normalized
    paths. Valid rules that do not match the current source/profile selection
    are allowed silently.

CONFIG-BACKED ENVIRONMENT VARIABLES

  OVERLAY_SOURCES    overrides sources (comma-separated)
  OVERLAY_TARGET     overrides target
  OVERLAY_PROFILES   overrides raw profiles (comma-separated)
  OVERLAY_CONTINUE   overrides continue_on_error

There is no OVERLAY_PROFILE. Use repeated --profile NAME flags for singular CLI
profile selection.

SOURCE RESOLUTION PRECEDENCE

Source roots are loaded from these sources, highest to lowest:

  1. positional command args for plan, diff, and render (e.g. overlay plan pi)
  2. --source / --sources CLI flags (--source adds one value; --sources accepts comma-separated values)
  3. OVERLAY_SOURCES env var
  4. .overlay.toml sources
  5. default ["."]

Positional sources are resolved relative to the config file directory when a
config file exists. This supports stow-style package selection:

  overlay --config ~/dotfiles/.overlay.toml plan pi codex

PROFILE RESOLUTION PRECEDENCE

Raw profiles are loaded from these sources, highest to lowest:

  1. --profiles CLI flag or repeated --profile NAME flags
  2. OVERLAY_PROFILES env var
  3. .overlay.toml profiles
  4. default []

When both CLI forms are used, --profiles values are applied first, then repeated
--profile values. After raw loading, Overlay appends the comma-split value of
the env var named by env_profiles (if set). Duplicates are removed, preserving
first occurrence.

Within the effective set, the layer order is always:
  base (if present) -> each profile in list order -> local (if present)

FILE CONVENTION

Overlay discovers files by either pattern:

  <stem>.olay.<profile>
  <stem>.olay.<profile>.<ext>

The stem is required and may contain dots. The profile name "base" is always
first, "local" is always last, and any other name is a user profile. If an
extension is present, it must be one filename segment with no additional dots.

By default, JSON, TOML, and YAML overlays are mergeable structured formats:

  dot-claude/settings.olay.base.json      -> base layer (always first)
  dot-claude/settings.olay.work.json      -> "work" profile layer
  dot-claude/settings.olay.local.json     -> local layer (always last)
  lazygit/config.olay.base.yml            -> YAML base layer
  lazygit/config.olay.dark.yml            -> YAML "dark" profile layer

With dot_prefix = true and target = "~/", those layers merge into:

  ~/.claude/settings.json

YAML inputs must be single-document config-style YAML. String-keyed mappings,
sequences, and scalar values are supported. Multi-document streams, non-string
or complex mapping keys, aliases, and custom tags are rejected with errors.

By default, valid overlays with any other extension, or no extension, are copied
through as whole files. The highest-precedence active layer wins:

  bin/tool.olay.base.sh
  bin/tool.olay.work.sh                  -> ~/bin/tool.sh copies work
  README.olay.local                      -> ~/README copies local

Malformed overlay-looking filenames with multi-part extensions are errors:

  archive.olay.work.tar.gz
  settings.olay.work.schema.json

With a stow-style config, the package directory is the source root and is not
part of the target path:

  sources = ["pi"]
  pi/dot-pi/agent/models.olay.base.json -> ~/.pi/agent/models.json

See README.md for a worked example.
`
