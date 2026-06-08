# YAML Merge Support Plan

## Intention

Add YAML as a first-class structured overlay format alongside JSON and TOML.

Users should be able to layer YAML configuration files by profile using the existing overlay naming and precedence model, then render a single deterministic YAML output file.

## Goals

- Treat `.yaml` and `.yml` overlays as mergeable by default.
- Reuse the existing Overlay merge contract:
  - maps deep-merge recursively
  - scalar lists concatenate with duplicate removal, preserving first-seen order
  - lists containing objects concatenate without deduplication
  - scalars and type mismatches are replaced by the higher-precedence layer
- Keep YAML output deterministic so `overlay diff` and golden-style checks remain trustworthy.
- Support common config-file YAML use cases, including a LazyGit-style base config plus theme/profile overlays.
- Fail early with clear errors for YAML shapes outside the supported config model.
- Preserve existing escape hatches: users can still force whole-file copy behavior with a `render_rules` entry.

## Non-goals

- Preserve YAML comments in rendered output.
- Preserve source formatting, key order, anchors, aliases, tags, or custom YAML styling.
- Support multi-document YAML streams.
- Support non-string or complex mapping keys.
- Add YAML formatting configuration in the first version.

## User-facing API

### File convention

YAML follows the existing overlay file convention:

```text
<stem>.olay.<profile>.yaml
<stem>.olay.<profile>.yml
```

Examples:

```text
config.olay.base.yml
config.olay.dark.yml
config.olay.local.yml
```

With active profile `dark`, these render to:

```text
config.yml
```

Layer order remains unchanged:

```text
base → active profiles in order → local
```

### Default render behavior

`.yaml` and `.yml` files merge by default, like `.json` and `.toml`.

Other extensions and extensionless overlays continue to use copy-through behavior unless configured otherwise.

### Render rules

Existing render rules continue to apply. A YAML target can be forced to copy-through behavior:

```toml
[[render_rules]]
path = ".config/example/config.yml"
strategy = "copy"
```

Rules still match the final target-relative path after `dot_prefix` mapping.

### Output contract

Rendered YAML should be stable and predictable:

- deterministic key ordering
- 2-space indentation
- block-style YAML suitable for config files
- no preservation of source comments or source-specific styling

### Input contract

Supported YAML inputs are config-style YAML documents:

- a single YAML document per overlay layer
- mappings with string keys
- sequences and scalar values suitable for the existing merge rules

Unsupported YAML should fail with an explanatory error instead of being silently coerced or partially ignored.

## Example scenarios

### Generic YAML merge

Base:

```yaml
app:
  name: overlay
  features:
    - json
    - toml
```

Profile:

```yaml
app:
  features:
    - toml
    - yaml
  debug: true
```

Expected rendered shape:

```yaml
app:
  debug: true
  features:
    - json
    - toml
    - yaml
  name: overlay
```

### LazyGit-inspired YAML merge

Base config:

```yaml
gui:
  nerdFontsVersion: "3"
filterMode: fuzzy
git:
  mainBranches:
    - main
    - develop
    - master
  pagers:
    - colorArg: always
      pager: delta --paging=never --line-numbers
```

Theme/profile overlay:

```yaml
gui:
  theme:
    activeBorderColor:
      - "#cba6f7"
      - bold
    selectedLineBgColor:
      - "#313244"
authorColors:
  "*": "#b4befe"
```

Expected outcome: the rendered file contains the base LazyGit config plus the selected theme values under `gui.theme` and `authorColors`.

## Validation plan

### Behavior checks

Validate that YAML behaves like other structured formats:

- `.yaml` merges by default.
- `.yml` merges by default.
- map values deep-merge.
- scalar lists dedupe and preserve first-seen order.
- object lists concatenate without deduplication.
- scalar overrides replace lower-precedence values.
- type mismatches use the higher-precedence value.
- `render_rules` can force YAML copy-through behavior.

### Strictness checks

Validate clear failure behavior for unsupported YAML inputs:

- multi-document YAML streams
- non-string mapping keys
- complex mapping keys
- invalid YAML syntax

### End-to-end checks

Validate through render-oriented examples:

- generic YAML base/profile/local layers render the expected YAML file.
- a compact LazyGit-inspired example renders a base config plus a selected theme overlay.
- `overlay plan` reports YAML targets as merge mode.
- `overlay diff` compares rendered YAML deterministically.

### Project readiness

Before considering the work complete, run:

```shell
make check
```

This remains the source of truth for readiness.
