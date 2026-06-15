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

  env_profiles = ["DOTFILES_PROFILE"]  # example
    type:    array of strings
    default: []
    Optional environment variable names. At runtime overlay reads each named
    env var in list order and APPENDS its comma-separated value to the
    'profiles' list (with duplicates removed, preserving first occurrence).
    Unset or empty-valued vars are skipped. Blank or whitespace-padded names
    are invalid.

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

    To opt a target out of variable substitution, list its path in
    substitute_exclude (below), not here.

    Without a matching render rule, defaults are unchanged:
      .json/.toml/.yaml/.yml -> merge
      other files -> copy

    Append preserves each active layer's content and appends in normal layer
    order: base, then active profiles, then local. It inserts one newline
    between adjacent non-empty layers only when the previous layer does not
    already end with a newline. It does not trim, de-duplicate, parse syntax, or
    force a final newline.

    Validation rejects missing or empty paths, absolute paths, paths
    containing "..", missing strategies, unsupported strategies, and duplicate
    normalized paths. Valid rules that do not match the current source/profile
    selection are allowed silently.

  substitute_prefixes = ["DOTFILES_THM_", "DOTFILES_THEME_"]   # example
    type:    array of strings
    default: []
    The variable-substitution switch. When non-empty, ${NAME} references in
    rendered output are replaced for every target (opt targets out with
    substitute_exclude). Each entry is a literal name prefix and must match
    [A-Za-z_][A-Za-z0-9_]*; only variables whose names start with a listed
    prefix are ever substituted. When empty (the default), substitution is
    fully off and output is byte-identical to prior overlay versions. See
    VARIABLE SUBSTITUTION.

  substitute_exclude = [".config/shell/**"]   # example
    type:    array of strings (doublestar glob patterns)
    default: []
    Opts matching targets out of substitution while it is globally on. Each
    pattern is matched against the rendered target-relative path (the same
    path render_rules match, e.g. ".config/shell/theme.sh"), not the walk
    path that the ignore field uses. ** matches any number of path segments.
    An exact path with no wildcards is a valid single-target exclusion. As
    with the ignore field, a pattern with no "/" matches by base name at any
    depth ("theme.sh" excludes ".config/shell/theme.sh"); add a "/" to anchor
    it. With substitute_prefixes empty the list is inert.

VARIABLE SUBSTITUTION

Reference syntax. Inside substituting content, ${NAME} is replaced with the
variable's value when NAME matches the POSIX name charset
[A-Za-z_][A-Za-z0-9_]* AND starts with a substitute_prefixes entry. Bare
$NAME is never substituted. Anything else — ${name:-default}, ${a.b},
${UNLISTED_PREFIX}, $HOME — passes through byte-identical, so files full of
shell or tool syntax stay untouched.

Escape. $${NAME} emits a literal ${NAME}. The escape is recognized exactly
where the reference would otherwise substitute: $$ alone, shell's $$ (PID),
and $${HOME} under a non-matching prefix all pass through unchanged. The
escape is only interpreted in substituting targets.

Opting out. A whole target can be excluded from substitution by listing its
rendered target-relative path (or a doublestar glob) in substitute_exclude;
an excluded target is not substituted, escapes included.

Composition order. For mergeable JSON/TOML/YAML targets, each active layer is
parsed first, then substitutions are applied to string values and mapping keys
inside that parsed layer, then the layers are merged and serialized. This lets
the target format quote and escape substituted strings safely. A missing
variable in any active layer fails the target, even if a later layer would
override that value; if key substitution creates duplicate keys within one map
in one layer, the target fails. For append and copy targets, substitution runs
once over the final composed bytes. Substituted values are never re-scanned: a
value containing ${OTHER} is emitted verbatim. Values are resolved from a
single environment snapshot taken once per invocation, so a run is
deterministic.

Values and pinning. Values come from the process environment. Pin values per
invocation with repeated --var NAME=value flags, the --vars A=1,B=2 flag, or
OVERLAY_VARS=A=1,B=2; pinned values win over the ambient environment. There
is deliberately no .overlay.toml key for pins: a committed pin would
permanently shadow the environment that substitution exists to consume.
Precedence follows the profiles convention:

  1. --vars A=1,B=2 then repeated --var NAME=value (later wins per name)
  2. OVERLAY_VARS=A=1,B=2 (fully replaced when any vars flag is set)
  3. ambient process environment

--vars and OVERLAY_VARS are comma-split, so values containing commas must
use --var. An exact duplicate NAME=value entry collapses to its first
position, so re-pinning a value you already passed will not override a
different value given in between. A pin whose name matches no
substitute_prefixes entry can never take effect and is an error; a prefixed
pin consumed by no target logs a warning. Pins affect content substitution
only — never env_profiles, never $VAR expansion in target/sources paths.

Errors. A reference to an unset variable fails the run; a variable set to
the empty string substitutes as empty. Render composes every target in
memory first: on failure it reports every failing target with all of its
missing variables and writes nothing (--continue writes the clean targets
and exits non-zero). diff exits 2 on missing variables. plan shows each
substituting target's variables in a VARS column, marks missing ones, and
exits non-zero — so failures are detectable from a dry run.

CONFIG-BACKED ENVIRONMENT VARIABLES

  OVERLAY_SOURCES    overrides sources (comma-separated)
  OVERLAY_TARGET     overrides target
  OVERLAY_PROFILES   overrides raw profiles (comma-separated)
  OVERLAY_CONTINUE   overrides continue_on_error
  OVERLAY_VARS       pins variables as NAME=value pairs (comma-separated);
                     there is no vars TOML key

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
each env var listed in env_profiles, in list order; unset or empty-valued vars
are skipped. Duplicates are removed, preserving first occurrence.

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

YAML inputs must be single-document config-style YAML with a root mapping.
Empty/comment-only YAML layers are accepted as no-op empty maps. Mapping values
may be sequences and scalar values. Root sequences/scalars, explicit root null,
multi-document streams, non-string or complex mapping keys, aliases, and custom
tags are rejected with errors.

JSON and YAML numeric output is normalized by their encoders; whole-valued
floats such as 1.0 may render as 1.

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
