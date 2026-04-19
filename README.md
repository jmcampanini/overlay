# overlay

A small Go CLI that merges layered JSON/TOML configuration files by profile.

`overlay` walks a source directory for files matching
`<stem>.olay.<profile>.<ext>`, groups them by stem, merges the matching
layers in order, and writes the result to a target directory. It's useful
for dotfiles layouts where you want a base file, a machine profile, and a
machine-local override to compose into a single rendered file.

## Quick start

```shell
# Build (output goes to out/overlay)
make build

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

## File convention

```
<stem>.olay.<profile>.<ext>
```

- `<ext>` is `json` or `toml`.
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

Output is deterministic: keys are alphabetized in both JSON and TOML so
`overlay diff` stays trustworthy and golden-file tests are stable.

## Profile resolution precedence

The active profile set is resolved from three possible sources. From
**highest to lowest** precedence:

1. `--profiles a,b,c` on the command line — replaces the entire set.
   Order on the command line is preserved.
2. `.overlay.toml` in the current directory (or `--config <path>`) —
   `config.profiles` first, then the comma-split value of the env var
   named by `config.env_profiles` (if set) is **appended**. Duplicates
   are removed, preserving first occurrence.
3. `OVERLAY_PROFILES` env var — used only when no `.overlay.toml` is
   found and no `--profiles` flag is given. Comma-split.

Within the resolved set, the merge layer order is always:

```
base → each profile in list order → local
```

### Worked example

```toml
# ./.overlay.toml
target = "~/"
profiles = ["base-tools"]
env_profiles = "DOTFILES_PROFILE"
```

```shell
# Scenario 1: config only → ["base-tools"]
overlay plan

# Scenario 2: config + env → ["base-tools", "work"]
DOTFILES_PROFILE=work overlay plan

# Scenario 3: CLI flag replaces → ["personal"]
DOTFILES_PROFILE=work overlay --profiles personal plan
```

Run `overlay config` to see which source each resolved value came from.

## Subcommands

| command | purpose |
|---|---|
| `overlay render` | Merge active layers and write output files to the target. |
| `overlay diff` | Print a git-style unified diff vs. the current target files. Exit 1 if any file differs. |
| `overlay plan` | Dry-run: print an aligned table of what would be generated. |
| `overlay config` | Print the fully-resolved configuration with per-field source annotations. `--validate <path>` schema-checks a file. |
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

Run `overlay docs` for the full schema including `source`, `dot_prefix`,
`env_profiles`, `continue_on_error`, `ignore`, `traverse_hidden`, and
`respect_gitignore`.

## Notes

- **JSON and TOML keys are alphabetized on output.** This is deliberate
  — it keeps `overlay diff` stable and makes golden-file tests cheap. If
  your source files had a hand-curated key order, it will not be
  preserved in the merged output.
- **Hidden directories are skipped by default.** Set
  `traverse_hidden = true` if you need to descend into `.foo` directories.
- **Symlinks are not followed** during the source walk in v1. See the
  roadmap below.

## Roadmap

- YAML support with the same merge semantics.
- Symlink following during source walk (with inode-based loop detection).

## Development

```shell
make             # list tasks (help is the default)
make build       # build ./out/overlay
make test        # go test -race ./...
make check       # fmt-check + tidy-check + lint + test
make clean       # remove ./out, coverage files, and test cache
```

The `.claude-sandbox/` directory is auto-created for local experiments
and is gitignored.
