# Render Rules Strategy Plan

## Goal

Add a small, explicit user-facing API for choosing how selected rendered files combine their active layers. The first target use case is an extensionless `.npmrc` where a base file and a work-profile file should be appended in layer order instead of copy-through replacement.

This should preserve Overlay's current defaults while allowing specific output files to opt into append or copy behavior.

## Outcomes

- `.npmrc` and other line-oriented text files can be layered by appending active profile files in order.
- JSON and TOML continue to merge by default.
- Non-JSON/TOML files continue to copy the highest-precedence active layer by default.
- Users can force copy-through behavior for a specific rendered target, including JSON/TOML targets.
- The dry-run plan clearly reports what will happen using a single user-facing mode: `merge`, `copy`, or `append`.
- Invalid render rule configuration is rejected early; valid rules that do not match the current run are allowed.

## Non-goals for v1

- No semantic `.npmrc` parser or key/value merge behavior.
- No configurable separators, trimming, final-newline policies, or line de-duplication.
- No glob or pattern matching for render rules.
- No source-relative render rule matching.
- No `strategy = "merge"` rule. Merge remains an extension-driven default for supported structured formats.
- No plugin or render-driver abstraction exposed to users.

## User-facing API

Render rules are configured in `.overlay.toml` with repeated `[[render_rules]]` tables.

Example:

```toml
target = "~/"
sources = ["npm"]
profiles = ["work"]

[[render_rules]]
path = ".npmrc"
strategy = "append"
```

### Fields

#### `path`

The rendered target-relative path to match exactly.

Examples:

```toml
path = ".npmrc"
path = ".ssh/config"
path = ".claude/settings.json"
```

The path is relative to the configured `target` directory after Overlay's normal target mapping, including `dot_prefix` behavior.

For example, with `dot_prefix = true`:

```text
npm/dot-npmrc.olay.base -> <target>/.npmrc
```

So this rule matches it:

```toml
[[render_rules]]
path = ".npmrc"
strategy = "append"
```

#### `strategy`

Allowed values:

```text
append
copy
```

No rule means Overlay uses its existing default behavior:

```text
.json/.toml -> merge
other files -> copy
```

## Behavior model

### Default behavior remains unchanged

Without a matching `render_rules` entry:

| File kind | Default mode |
|---|---|
| JSON | `merge` |
| TOML | `merge` |
| Other extension or extensionless | `copy` |

### `strategy = "append"`

Active layers are appended in normal Overlay layer order:

```text
base -> each active profile in order -> local
```

Append is boundary-safe for text files:

- preserve each layer's content,
- append active layers in order,
- insert one `\n` between adjacent non-empty layers only when the previous layer does not already end with `\n`,
- do not trim content,
- do not de-duplicate lines,
- do not parse file-specific syntax,
- do not force a final newline beyond what the layer contents and boundary rule produce.

Example source layout:

```text
npm/
├── dot-npmrc.olay.base
└── dot-npmrc.olay.work
```

Base:

```ini
allow-git=root
audit=true
ignore-scripts=true
min-release-age=7
package-lock=true
save-exact=true
yes=false
```

Work:

```ini
@company:registry=https://registry.example.com/
//registry.example.com/:_authToken=${WORK_NPM_TOKEN}
```

Rendered with the `work` profile:

```ini
allow-git=root
audit=true
ignore-scripts=true
min-release-age=7
package-lock=true
save-exact=true
yes=false
@company:registry=https://registry.example.com/
//registry.example.com/:_authToken=${WORK_NPM_TOKEN}
```

### `strategy = "copy"`

The highest-precedence active layer wins.

This is already the default for non-JSON/TOML files. The explicit rule is useful when a JSON or TOML target should be treated as a whole-file overlay instead of being structurally merged.

Example:

```toml
[[render_rules]]
path = ".some/generated.json"
strategy = "copy"
```

## Plan output

`overlay plan` should show the user-facing render mode instead of the lower-level file format.

Current conceptual columns:

```text
TARGET  FORMAT  LAYERS
```

Desired columns:

```text
TARGET  MODE  LAYERS
```

Mode values:

```text
merge
copy
append
```

Examples:

```text
TARGET                    MODE    LAYERS
~/.npmrc                  append  base, work
~/.claude/settings.json   merge   base, work
~/bin/tool.sh             copy    base, work (winner: work)
```

For copy mode, the plan should continue to identify the winning layer.

## Validation rules

Render rule validation should be strict for invalid configuration.

Invalid cases should fail validation:

- missing `path`,
- empty `path`,
- absolute `path`,
- path traversal such as `..`,
- unsupported strategy value,
- missing `strategy`,
- duplicate normalized `path` entries.

Valid unmatched rules should be allowed silently.

This matters because Overlay supports package/source selection. A global config may contain:

```toml
[[render_rules]]
path = ".npmrc"
strategy = "append"
```

But a run such as:

```shell
overlay plan pi
```

may not include the `npm` source package. That should not be an error or warning.

## Testing expectations

Tests should focus on externally observable behavior and user-facing contracts.

### Defaults remain stable

Verify existing behavior remains unchanged when no render rule matches:

- JSON layers merge.
- TOML layers merge.
- extensionless files copy the highest-precedence active layer.
- unknown/non-structured extensions copy the highest-precedence active layer.

### Append behavior

Verify `strategy = "append"`:

- appends active layers in `base -> profiles -> local` order,
- respects profile ordering,
- omits inactive profile layers,
- handles base-only, profile-only, and local-only cases,
- preserves layer contents,
- inserts a newline only when needed between adjacent non-empty layers,
- does not add unnecessary blank lines,
- handles empty layers predictably,
- produces identical output through render and diff paths.

Important newline cases:

| Base ends with newline | Work starts non-empty | Expected |
|---|---|---|
| yes | yes | no extra inserted newline |
| no | yes | one inserted newline |
| yes | empty | no extra content |
| empty | yes | work content only |

### Copy override behavior

Verify `strategy = "copy"` can force a normally mergeable JSON or TOML target to copy the winning layer rather than merge layers.

### Rule matching

Verify rules match rendered target-relative paths:

- `dot-npmrc.olay.base` matches `path = ".npmrc"` when `dot_prefix = true`,
- nested targets such as `.ssh/config` match as target-relative paths,
- source-relative names such as `dot-npmrc` are not the matching API,
- valid rules that do not match the current active groups are ignored silently.

### Validation

Verify config validation rejects:

- empty paths,
- absolute paths,
- paths containing `..`,
- duplicate normalized paths,
- missing strategies,
- unsupported strategies such as `merge`, `auto`, or `replace`.

Verify supported strategies are exactly:

```text
append
copy
```

### Plan output

Verify `overlay plan` reports `MODE`, not `FORMAT`, and displays:

- `merge` for default JSON/TOML merge groups,
- `copy` for default copy-through groups,
- `append` for matching append rules,
- copy winner annotation for copy mode.

### Documentation

Verify `overlay docs` includes:

- the `[[render_rules]]` table,
- `path` semantics as rendered target-relative path,
- supported strategies,
- default behavior when no rule matches,
- append newline behavior,
- validation constraints,
- unmatched-rule behavior.

## Validation workflow

Before considering the feature ready:

1. Validate example config:

   ```shell
   overlay config --validate .overlay.toml
   ```

2. Confirm dry-run mode selection:

   ```shell
   overlay --profiles work plan npm
   ```

3. Confirm rendered diff before writing:

   ```shell
   overlay --profiles work diff npm
   ```

4. Render only after the plan and diff match expectations:

   ```shell
   overlay --profiles work render npm
   ```

5. Run the project readiness check:

   ```shell
   make check
   ```

## Example final `.npmrc` setup

Config:

```toml
target = "~/"
sources = ["npm"]

[[render_rules]]
path = ".npmrc"
strategy = "append"
```

Files:

```text
npm/dot-npmrc.olay.base
npm/dot-npmrc.olay.work
```

Run:

```shell
overlay --profiles work render npm
```

Expected result:

```text
~/.npmrc = base contents + work contents
```

The same source files without the render rule would keep current Overlay behavior:

```text
~/.npmrc = work contents only
```
