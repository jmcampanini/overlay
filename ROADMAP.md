# ROADMAP

## TOML style directives from source comments

### Problem

Overlay merges TOML through generic maps, which makes output deterministic but loses source-level style intent. This is noticeable for configs that prefer mixed TOML forms, for example keeping top-level `[[actions]]` blocks while preserving nested `stages` as inline-table arrays.

Current serializer options are too coarse:

- `SetTablesInline(false)` keeps `[[actions]]`, but expands nested `stages` to `[[actions.stages]]`.
- `SetTablesInline(true)` keeps `stages` inline, but collapses `actions` into one large inline array.

Neither produces the desired mixed style.

### Proposed solution

Allow source TOML comments to declare formatting hints that Overlay carries through merge and applies during serialization.

Example directive:

```toml
# overlay: inline-table-array actions[].stages

[[actions]]
name = "open html"
stages = [
  { type = "picker", source = "fd --extension html", key = "picked" },
]
```

The directive means:

- keep `actions` as normal array-table blocks: `[[actions]]`
- serialize `actions[].stages` as `stages = [{ ... }]`

### Implementation shape

1. Pre-scan active TOML source layers for `# overlay:` directives before normal parsing.
2. Merge style directives using the same layer precedence as data: `base -> profiles -> local`.
3. Store style rules alongside the merged document for serialization.
4. Teach the TOML emitter to check the current path while walking the merged value tree.
5. When a path matches `inline-table-array`, emit that array as inline table elements instead of nested `[[...]]` blocks.

Target output shape:

```toml
[[actions]]
name = "open html"
matches = "root"
cmd = "open {{sq .picked}}"
icon = ":nf-dev-html5:"
stages = [
  { type = "picker", source = "fd --extension html", key = "picked" },
]
```

### Notes

- This is a formatting feature only; merge semantics should not change.
- Key ordering can remain deterministic for now.
- Comments are preferred over hidden metadata keys because they do not become part of the target config.
