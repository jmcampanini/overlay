# Multiple Sources / Stow-Style Package Roots Plan

## Context

The current overlay path mapping treats the configured `source` directory as the root of the rendered target tree. That is correct for layouts like:

```text
source/dot-claude/settings.olay.base.json -> ~/.claude/settings.json
```

But it does not mirror GNU Stow's package model. In a stow directory, the first path component is the package name and is not part of the target path:

```text
stow-dir/pi/dot-pi/agent/models.json -> ~/.pi/agent/models.json
```

For overlay, the equivalent should be that each package directory is its own source root:

```text
sources = ["pi"]

pi/dot-pi/agent/models.olay.base.json -> ~/.pi/agent/models.json
```

This avoids a `strip_components` knob and keeps overlay's existing rule intact: paths are rendered relative to the source root.

## Goal

Add first-class support for multiple source roots so a stow-style dotfiles repo can configure package directories directly.

Target UX:

```toml
# .overlay.toml at the stow/dotfiles repo root
sources = [
  "claude-code",
  "cmdk",
  "codex",
  "opencode",
  "pi",
]

target = "~/"
dot_prefix = true
env_profiles = "DOTFILES_PROFILE"
```

Then:

```shell
overlay plan
overlay diff
overlay render
```

walk all configured package roots and render each file relative to its package directory, not relative to the repo root.

## Stow mirror

Stow is invoked with package names:

```shell
stow pi codex
```

Overlay should eventually support the same mental model:

```shell
overlay plan pi codex
overlay diff pi
overlay render pi codex
```

Those positional arguments are source/package selectors for the subcommand. They replace the configured `sources` list for that invocation.

This gives two modes:

1. **Configured all-packages mode** — `overlay render` uses every source in `.overlay.toml`.
2. **Stow-like selected-package mode** — `overlay render pi codex` uses only those package roots.

## Config schema

Keep backwards compatibility with existing singular `source`:

```toml
source = "."       # existing single-source form
sources = ["pi"]   # new multi-source form
```

Rules:

- `source` remains supported.
- `sources` is added as `[]string`.
- A config file may set either `source` or `sources`, but not both.
- If neither is set, the default remains `source = "."`.
- The resolved runtime representation should be a list of source dirs.
- Relative source paths from config are resolved relative to the config file's directory, as `source` is today.

Recommended resolved config output:

```toml
sources = ["/abs/path/to/pi", "/abs/path/to/codex"] # from: config
```

Even if the user configured singular `source`, `overlay config` can print the normalized `sources` list for clarity.

## CLI behavior

### Flags

Current flag:

```shell
--source <dir>
```

Should become repeatable / comma-aware while preserving existing usage:

```shell
--source pi
--source pi --source codex
--source pi,codex
```

Implementation can use Cobra `StringSliceVar` for `--source`.

`--source` continues to replace config sources, matching the existing override behavior.

### Positional source/package args

For `render`, `diff`, and `plan`:

```text
overlay render [source...]
overlay diff [source...]
overlay plan [source...]
```

Resolution precedence for sources should be:

1. Positional subcommand args, if present.
2. `--source`, if present.
3. `.overlay.toml` `sources` or `source`.
4. Default `source = "."`.

For stow-like use, positional args should be resolved relative to the config file directory when a config file exists. This lets a user run:

```shell
overlay --config ~/dotfiles/.overlay.toml plan pi codex
```

and have `pi` mean `~/dotfiles/pi`, not `$PWD/pi`.

## Discovery semantics

Change discovery from one source root to many source roots.

For each source root:

1. Walk that source root independently.
2. Compute `relDir` relative to that source root.
3. Build target path from `target + relDir + stem.ext`.
4. Apply `dot_prefix` exactly as today.

Example:

```text
source root: /dotfiles/pi
source file: /dotfiles/pi/dot-pi/agent/models.olay.base.json
relDir:      dot-pi/agent
target:      ~/.pi/agent/models.json
```

This is the core reason multi-source roots mirror Stow cleanly: the package directory is the root, not a path component to strip.

## Collision behavior

Target path collision detection must become global across all sources.

Example collision:

```text
sources = ["pi", "other-pi"]

pi/dot-pi/agent/models.olay.base.json
other-pi/dot-pi/agent/models.olay.base.json
```

Both produce:

```text
~/.pi/agent/models.json
```

Overlay should fail with a clear error naming both source files.

## Internal design

### `internal/config`

- Add `Sources []string 'toml:"sources"'`.
- Preserve `Source string 'toml:"source"'`.
- Extend validation:
  - reject explicit `source` + explicit `sources` in the same file;
  - reject empty strings in `sources`;
  - keep unknown-key rejection.
- Update `SchemaDocs` with `sources` and the compatibility note for `source`.

### `internal/cli`

- Change `GlobalFlags.Source` from `string` to `[]string`.
- Bind `--source` with `StringSliceVar`.
- Add source provenance, likely still one field: `Sources`.
- Add source resolution helper that returns `[]string`.
- Update `overlay config` output to print normalized `sources`.
- Update help for `render`, `diff`, and `plan` to show `[source...]` args and explain stow-style package selection.

### `internal/discover`

Preferred shape:

```go
type Settings struct {
    SourceDirs []string
    TargetDir string
    DotPrefix bool
    Profiles []string
    Ignore Ignorer
    TraverseHidden bool
    RespectGitignore bool
}
```

`Walk(Settings)` should loop over `SourceDirs` internally and return one sorted combined group list.

Group sort order should remain deterministic. Suggested order:

1. source root path;
2. relative directory;
3. stem;
4. extension.

`Group` may optionally gain `SourceDir string` for better plan output and diagnostics.

### `internal/render`, `internal/diff`, `internal/plan`

- Update empty-discovery logs from singular source to plural sources.
- Update plan header from:

```text
Source: ./  Target: ~/
```

to:

```text
Sources: pi, codex, opencode  Target: ~/
```

or for many sources:

```text
Sources: 5 configured  Target: ~/
```

## Dotfiles migration example

Current problematic single-source config:

```toml
source = "."
target = "~/"
dot_prefix = true
env_profiles = "DOTFILES_PROFILE"

ignore = [
  "firefox/**",
  "homebrew/**",
  "keyboard/**",
  "one-off/**",
  "research/**",
  "__pycache__/**",
  ".generated/**",
]
```

New stow-style config:

```toml
sources = [
  "claude-code",
  "cmdk",
  "codex",
  "opencode",
  "pi",
]

target = "~/"
dot_prefix = true
env_profiles = "DOTFILES_PROFILE"

ignore = [
  "__pycache__/**",
]
```

The package names are now explicit, so the old package-exclusion ignore list is no longer needed for overlay-managed files.

## Tests

Add/adjust tests for:

- single `source` still works;
- `sources = [...]` walks each root relative to itself;
- `pi/dot-pi/...` maps to `~/.pi/...` with `sources = ["pi"]`;
- `source` + `sources` in the same config errors;
- `--source pi --source codex` replaces config sources;
- `overlay plan pi codex` replaces config sources;
- target collisions across different source roots error;
- plan output displays multiple sources clearly;
- `overlay config` prints normalized sources and correct provenance;
- relative config sources resolve relative to the config file directory.

## Validation

Use project-standard validation:

```shell
make check
```

Add a smoke test under `.claude-sandbox/multiple-sources-stow/` that creates:

```text
repo/.overlay.toml
repo/pi/dot-pi/agent/models.olay.base.json
```

and confirms:

```shell
overlay --config repo/.overlay.toml plan
```

plans:

```text
~/.pi/agent/models.json
```

not:

```text
~/pi/.pi/agent/models.json
```

## Non-goals

- Do not add `strip_components` in this approach.
- Do not add stow-specific `.stowrc` parsing.
- Do not infer packages by scanning every top-level directory yet. Keep package/source selection explicit.
- Do not follow symlinks; keep current v1 behavior.
