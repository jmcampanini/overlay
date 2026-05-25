# PLAN: Copy-through overlays for non-merge files

## Purpose

Extend Overlay so files that use the `olay` profile naming convention can be rendered even when they are not JSON or TOML.

JSON and TOML remain mergeable structured formats. Other valid overlay files are copied through as whole files using the same profile selection model.

## Desired outcome

Users can keep profile-specific files such as scripts, YAML files, text files, and extensionless files alongside existing JSON/TOML overlays:

```text
bin/tool.olay.base.sh
bin/tool.olay.work.sh
config.olay.work.yaml
README.olay.local
```

and render them to:

```text
bin/tool.sh
config.yaml
README
```

using the existing commands:

```shell
overlay plan
overlay diff
overlay render
```

## User-facing API

### Filename convention

Valid overlay filenames are:

```text
<stem>.olay.<profile>
<stem>.olay.<profile>.<ext>
```

Where:

- `<stem>` is required and may contain dots.
- `<profile>` is required.
- `<ext>` is optional.
- If present, `<ext>` must be a single filename segment with no additional dots.

Examples:

```text
README.olay.work              -> README
script.olay.base.sh           -> script.sh
settings.schema.olay.work.json -> settings.schema.json
```

Invalid examples:

```text
archive.olay.work.tar.gz      # multi-part extension
settings.olay.work.schema.json # multi-part extension
.olay.work.sh                 # missing stem
script.olay..sh               # missing profile
```

### Mergeable formats

The fixed mergeable format list remains:

```text
json
toml
```

A valid overlay file whose extension is `json` or `toml` is merged using existing Overlay merge behavior.

Examples:

```text
settings.olay.base.json
settings.olay.work.json
-> settings.json
-> merge base + work
```

```text
settings.schema.olay.base.json
settings.schema.olay.work.json
-> settings.schema.json
-> merge base + work
```

### Copy-through formats

A valid overlay file with any non-merge extension, or with no extension, is copied through as a whole file.

Examples:

```text
script.olay.base.sh
script.olay.work.sh
-> script.sh
-> copy the highest-precedence active layer
```

```text
config.olay.work.yaml
-> config.yaml
-> copy work
```

```text
README.olay.local
-> README
-> copy local
```

No new config field, flag, or command is introduced for this behavior.

## Profile and layer behavior

Copy-through files use the same active profile model as mergeable files:

```text
base -> active profiles in configured order -> local
```

For mergeable files, all active layers are merged in that order.

For copy-through files, active layers are not combined. The highest-precedence active layer wins, meaning the last active layer in Overlay order is copied.

Example with active profile `work`:

```text
tool.olay.base.sh
tool.olay.work.sh
tool.olay.local.sh
-> tool.sh
-> copy local
```

If `local` is absent:

```text
tool.olay.base.sh
tool.olay.work.sh
-> tool.sh
-> copy work
```

If only `base` is active:

```text
tool.olay.base.sh
-> tool.sh
-> copy base
```

A `base` layer is not required.

## Render expectations

`overlay render` writes materialized target files.

For copy-through files:

- content is copied from the winning source layer
- source permissions are not preserved
- symlinks are not created
- output files use the same inert write semantics as current JSON/TOML render output

This keeps copy-through behavior consistent with existing rendered outputs.

## Diff expectations

`overlay diff` compares the target file content against the content that would be rendered.

For copy-through files, the comparison is between:

- the existing target file content
- the winning source layer content

Permissions and symlink metadata are not part of diff output.

## Plan expectations

`overlay plan` includes copy-through files in the normal plan table.

For mergeable files, the format remains the structured format:

```text
settings.json  json  base, work, local
```

For copy-through files, the format is shown as `copy`, and the layer display shows both the active layers and the winner:

```text
tool.sh  copy  base, work (winner: work)
```

This makes it clear that copy-through files use the profile stack for selection, not composition.

## Collision and error expectations

Existing target collision behavior remains: if more than one active overlay output resolves to the same target path, Overlay reports an error instead of choosing a winner implicitly.

Malformed overlay-looking filenames with multi-part extensions are errors. This prevents ambiguous target names and keeps the convention predictable.

## Design decisions

1. JSON and TOML remain the only mergeable formats.
2. Valid non-JSON/TOML overlay files are copied through as whole files.
3. Extensionless overlay files are valid.
4. Multi-part extensions after the profile are invalid.
5. Copy-through files use existing profile order for precedence.
6. The highest-precedence active copy-through layer wins.
7. Copy-through writes materialized files, not symlinks.
8. Copy-through does not preserve source permissions.
9. No new user configuration is introduced for merge/copy format selection.
10. `plan`, `diff`, and `render` all support copy-through files.
11. `plan` explicitly identifies copy-through winners.
12. Existing target collision errors remain in place.

## Non-goals for this phase

This phase does not include:

- configurable merge extension lists
- configurable copy extension allowlists
- compound extensions such as `.tar.gz`
- symlink rendering
- permission preservation
- binary-specific diff behavior
- per-file render strategies
- content transforms for copied files
- whole-file override of JSON/TOML merge outputs

## Success criteria

This feature is successful when a user can:

1. Add valid non-JSON/TOML overlay files using the normal profile naming convention.
2. Render those files to the expected target paths.
3. Rely on existing profile precedence to choose the winning copy-through layer.
4. Preview copy-through decisions with `overlay plan`, including the selected winner.
5. Review copy-through content changes with `overlay diff`.
6. Keep JSON/TOML merge behavior unchanged.
7. Get clear errors for invalid multi-part extension filenames.
