# PLAN: Append-only Markdown overlays

## Purpose

Extend Overlay so Markdown files can participate in the same profile-based overlay workflow as JSON and TOML, using simple append-only composition.

This is intended to make Markdown useful for generated instructions, notes, README variants, and profile-specific documentation without introducing Markdown parsing or semantic section merging.

## Desired outcome

Users can create Markdown overlay layers such as:

```text
README.olay.base.md
README.olay.work.md
README.olay.local.md
```

and render them to:

```text
README.md
```

using the existing commands:

```shell
overlay plan
overlay diff
overlay render
```

Markdown should feel like a first-class supported Overlay format while keeping its behavior intentionally simple.

## User-facing interface

### Supported extension

Only `.md` is supported initially.

Supported:

```text
README.olay.base.md
```

Not supported initially:

```text
README.olay.base.markdown
README.olay.base.mdown
README.olay.base.mkdn
```

### Filename convention

Markdown uses the existing Overlay filename convention:

```text
<stem>.olay.<profile>.md
```

Examples:

```text
README.olay.base.md        -> README.md
docs/guide.olay.work.md    -> docs/guide.md
dot-github/AGENTS.olay.md  -> invalid; profile is required
```

The existing `dot-` path behavior continues to apply where configured.

### Layer ordering

Markdown uses the same profile order as JSON and TOML:

```text
base -> active profiles in configured order -> local
```

For example:

```shell
overlay --profiles work,mac render
```

with:

```text
README.olay.base.md
README.olay.work.md
README.olay.mac.md
README.olay.local.md
```

composes in this order:

```text
base
work
mac
local
```

### Missing `base`

A `base` layer is not required.

If any active Markdown layer exists, Overlay may produce the target file. For example, with profile `work` active:

```text
README.olay.work.md -> README.md
```

is valid.

### Command behavior

Markdown participates in the existing commands:

- `overlay plan` shows Markdown targets.
- `overlay diff` compares rendered Markdown output to existing target files.
- `overlay render` writes rendered Markdown output.

There is no new Markdown-specific command or flag for this MVP.

## Composition behavior

Markdown composition is append-only.

Active layers are read in overlay order and joined into one output document.

### Separator policy

Overlay inserts one blank line between non-empty Markdown layers.

Conceptually:

```text
<base content>

<profile content>

<local content>
```

This avoids accidental Markdown collisions such as a paragraph and heading being joined on the same line.

### Preservation policy

Overlay preserves each Markdown layer's content as written, except for the separator inserted between non-empty layers.

Overlay does not reformat Markdown, normalize headings, parse sections, reorder content, or trim intentional document content.

### Validation policy

Overlay does not validate Markdown for this MVP.

Markdown layers are treated as text fragments. Invalid or unusual Markdown is passed through rather than rejected.

## Design decisions

1. Markdown support is append-only for now.
2. Append-only is treated as the initial internal Markdown strategy, but no public `markdown_strategy` configuration is introduced yet.
3. Only `.md` is supported initially.
4. Markdown uses the existing `<stem>.olay.<profile>.<ext>` convention.
5. Markdown uses the existing layer order: `base -> profiles -> local`.
6. Markdown does not require a `base` layer.
7. Markdown participates fully in `plan`, `diff`, and `render`.
8. Markdown content is preserved except for blank-line separators between non-empty layers.
9. Markdown is not parsed or validated.

## Non-goals for this phase

This phase does not include:

- front matter merging
- heading-aware or section-aware merging
- block directives
- Markdown AST parsing
- Markdown linting
- configurable Markdown strategies
- configurable Markdown extensions
- per-file composition options
- automatic table/list/code formatting

These may be considered later if append-only composition proves insufficient.

## Success criteria

Markdown support is successful when a user can:

1. Add `.md` overlay layers using the normal filename convention.
2. Preview Markdown outputs with `overlay plan`.
3. Review Markdown changes with `overlay diff`.
4. Generate Markdown targets with `overlay render`.
5. Rely on predictable layer ordering and blank-line separation.
6. Keep authored Markdown content intact without parser-driven formatting changes.
