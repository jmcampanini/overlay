# overlay

A small Go CLI that merges layered JSON/TOML/YAML configuration files by profile
and renders other profile-specific files as whole-file overlays.

`overlay` walks one or more source directories for files matching
`<stem>.olay.<profile>[.<ext>]`, groups them by target path, renders the
active layers, and writes the result to a target directory. By default,
JSON/TOML/YAML layers are merged; other extensions and extensionless overlays
copy the highest precedence active layer. It's useful for dotfiles layouts where you
want a base file, a machine profile, and a machine-local override to compose
into a single rendered file.

## Install

### Homebrew `--HEAD` (macOS)

```shell
brew tap jmcampanini/overlay https://github.com/jmcampanini/overlay
brew install --HEAD jmcampanini/overlay/overlay
```

Upgrade to the latest commit:

```shell
brew upgrade --fetch-HEAD overlay
```

### From source

```shell
make build
# binary at ./build/overlay — copy or symlink onto your PATH
```

## Quick start

```shell
# Example layout
src/
├── .overlay.toml
└── dot-claude/
    ├── settings.olay.base.json      # always merged first
    ├── settings.olay.work.json      # "work" profile
    └── settings.olay.local.json     # always merged last

# Run
cd src
overlay --profiles work render
```

With `dot_prefix = true` (the default) and `target = "~/"`, the merged
file is written to `~/.claude/settings.json`.

For stow-style dotfiles repos, configure package directories as separate
source roots:

```toml
sources = ["pi", "codex", "opencode"]
target = "~/"
dot_prefix = true
```

Then `pi/dot-pi/agent/models.olay.base.json` renders to
`~/.pi/agent/models.json`; the package directory (`pi`) is the source root and
does not appear in the target path. Missing source roots are skipped with a
warning; existing roots with no overlay files are no-ops. You can select
packages for one run with positional args:

```shell
overlay plan pi codex
overlay diff pi
overlay render pi codex
```

## File convention

```
<stem>.olay.<profile>
<stem>.olay.<profile>.<ext>
```

- `<ext>` is optional. If present, it must be a single filename segment.
- By default, `json`, `toml`, `yaml`, and `yml` overlays are merged; every other
  extension, plus extensionless overlays, is copied through as a whole file.
- `<profile>` names the layer. `base` and `local` are **reserved**:
  `base` always merges first, `local` always merges last, with any active
  user profiles in between. User profiles can be named anything else.
- `base` is optional; if absent the merge starts from an empty document.
- Path segments beginning with `dot-` are rewritten to leading dots in
  the output path when `dot_prefix` is enabled (e.g. `dot-claude` → `.claude`).

## Merge semantics

- **Maps** are deep-merged recursively. Keys from the override overwrite
  matching keys in the base; nested maps recurse.
- **Lists of scalars** (strings, numbers, bools) are concatenated with
  duplicates removed, preserving first-seen order.
- **Lists containing objects** are concatenated without deduplication.
- **Scalars and type mismatches**: the override wins.

For copy-through files, active layers are not combined. The last active layer
in precedence order wins, so `base -> work -> local` copies `local`.

`[[render_rules]]` entries in `.overlay.toml` can opt a specific rendered
target-relative path into `append` or `copy` behavior:

```toml
[[render_rules]]
path = ".npmrc"
strategy = "append"
```

Rules match the final target-relative path after `dot_prefix` mapping, so
`dot-npmrc.olay.base` matches `path = ".npmrc"`. `append` concatenates active
layers in Overlay order, inserting one newline between adjacent non-empty layers
only when needed. `copy` forces whole-file copy behavior, including for JSON,
TOML, or YAML targets that would otherwise merge.

Output is deterministic: keys are alphabetized in JSON, TOML, and YAML so
`overlay diff` stays trustworthy and golden-file tests are stable. JSON and YAML
numeric output is normalized; whole-valued floats such as `1.0` may render as
`1`.

## Profile resolution precedence

Raw config is loaded from defaults, `.overlay.toml`, `OVERLAY_*` environment
variables, and config-backed flags, with later sources overriding earlier ones.
For profiles, the raw `profiles` value is chosen from these sources, highest to
lowest:

1. `--profiles a,b,c` or repeated `--profile a --profile b` on the command line.
2. `OVERLAY_PROFILES=a,b,c` in the environment.
3. `profiles = [...]` in `.overlay.toml`.
4. the default empty list.

`--profile` is a pflag-only singular alias: there is no `profile` TOML key and
no `OVERLAY_PROFILE` environment variable. When `--profiles` and `--profile` are
both used, `--profiles` values are applied first, then repeated `--profile`
values. Duplicates are removed, preserving first occurrence.

After raw loading, Overlay appends the comma-split value of each env var listed
in `env_profiles`, in list order; unset or empty-valued vars are skipped.
Duplicates are removed, preserving first occurrence.

Within the effective set, the merge layer order is always:

```
base → each profile in list order → local
```

### Worked example

```toml
# ./.overlay.toml
target = "~/"
profiles = ["base-tools"]
env_profiles = ["DOTFILES_PROFILE", "HOST_PROFILE"]
```

```shell
# Scenario 1: config only → ["base-tools"]
overlay plan

# Scenario 2: config + env → ["base-tools", "work"]
DOTFILES_PROFILE=work overlay plan

# Scenario 3: CLI flag sets raw profiles, then env_profiles appends
# → ["personal", "work"]
DOTFILES_PROFILE=work overlay --profiles personal plan

# Scenario 4: repeated --profile flags set raw profiles in order
# → ["personal", "client"]
overlay --profile personal --profile client plan

# Scenario 5: each env var appends in list order, duplicates keep their
# first occurrence → ["base-tools", "work", "laptop"]
DOTFILES_PROFILE=work,laptop HOST_PROFILE=laptop overlay plan
```

Run `overlay config` to see loaded values, GoConfigLoader provenance, and commented effective runtime values.

## Subcommands

| command | purpose |
|---|---|
| `overlay render` | Merge active layers and write output files to the target. |
| `overlay diff` | Print a git-style unified diff vs. the current target files. Exit 1 if any file differs. |
| `overlay plan` | Dry-run: print an aligned table of what would be generated. |
| `overlay config` | Print loaded configuration, GoConfigLoader provenance, and effective runtime comments. `--validate <path>` validates the effective runtime config for a file using env vars and config-backed flags. |
| `overlay docs` | Print the full `.overlay.toml` schema reference. |

For the complete config schema run `overlay docs`. Every subcommand
documents profile resolution in its `--help` output.

### Diff output and pipes

`overlay diff` prints standard unified diff to stdout and exits:

- **0** — all target files match the rendered output.
- **1** — at least one file differs.
- **2** — resolution or I/O error.

Pipe it into any diff viewer:

```shell
overlay diff | delta
overlay diff | bat --language=diff
overlay diff | git diff --no-index --color /dev/null /dev/stdin
overlay diff | diff-so-fancy
```

## Configuration

A minimal `.overlay.toml`:

```toml
target = "~/"              # required
profiles = ["work"]        # optional
```

Run `overlay docs` for the full schema including `sources`, `dot_prefix`,
`env_profiles`, `continue_on_error`, `toml_indent_tables`, `ignore`,
`traverse_hidden`, `respect_gitignore`, and `render_rules`.

Config-backed environment variables are `OVERLAY_SOURCES`, `OVERLAY_TARGET`,
`OVERLAY_PROFILES`, and `OVERLAY_CONTINUE`. There is no `OVERLAY_PROFILE`;
`--profile` is CLI-only. `sources` and `target` path expansion (`~`, `$VAR`, and
config-file-relative TOML paths) happens at runtime; the `overlay config` reports
the loaded strings and comments the expanded effective paths.

## Notes

- **JSON, TOML, and YAML keys are alphabetized on output.** This is deliberate
  — it keeps `overlay diff` stable and makes golden-file tests cheap. If
  your source files had a hand-curated key order, it will not be
  preserved in the merged output.
- **YAML input and output are normalized.** YAML merge inputs must be
  single-document root mappings; empty/comment-only YAML layers are accepted as
  no-op empty maps. Rendered YAML uses deterministic block style with 2-space
  indentation. Comments, source formatting, and source key order are not
  preserved; root sequences/scalars, explicit root null, multi-document streams,
  non-string or complex mapping keys, aliases, and custom tags are rejected.
- **TOML tables are unindented by default.** Set `toml_indent_tables = true`
  to ask the TOML encoder to indent nested tables and array-table values.
- **Hidden directories are skipped by default.** Set
  `traverse_hidden = true` if you need to descend into `.foo` directories.
- **Symlinks are not followed** during the source walk in v1. See the
  roadmap below.

## Roadmap

- Symlink following during source walk (with inode-based loop detection).

## Development

```shell
make             # list tasks (help is the default)
make build       # build ./build/overlay
make test        # go test -race ./...
make check       # fmt-check + tidy-check + lint + test
make clean       # remove ./build, coverage files, and test cache
```

The `.claude-sandbox/` directory is auto-created for local experiments
and is gitignored.
