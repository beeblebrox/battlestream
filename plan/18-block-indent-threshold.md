# 18 — [BUG] `reBlockTag` hard-codes 4-space indent — breaks on Blizzard format change

**Priority:** LOW
**Status:** DONE
**Area:** `internal/parser/parser.go`

**Resolution:** Extracted magic number to `blockTagMinIndent = 4` constant. Added
`slog.Warn` in `flushPending` when a block is flushed with empty Tags (signals indent
regex mismatch).

## Problem

The regex `-\s{4,}tag=` hard-codes a minimum indent of 4 spaces. If Blizzard changes
the indentation depth in a future log format update, continuation lines will not be
recognised and multi-tag entity blocks will emit incomplete events (missing ATK, HEALTH,
ZONE, etc.) without any warning. The failure is silent — the block is flushed with an
empty `Tags` map.

## Fix

### Option A: Make the indent threshold configurable

```go
const blockTagMinIndent = 4 // spaces; update if Blizzard changes log format

var reBlockTag = regexp.MustCompile(fmt.Sprintf(`^\s{%d,}tag=`, blockTagMinIndent))
```

At minimum this makes the magic number visible and easy to update.

### Option B: Accept any non-zero leading whitespace

```go
var reBlockTag = regexp.MustCompile(`^\s+tag=`)
```

This is more permissive and tolerant of any indentation depth, but risks matching
tag-like content at unexpected indentation levels. Review the log format to confirm
this is safe.

### Companion: warn on empty Tags flush

Regardless of which option is chosen, add a warning when a `FULL_ENTITY` block is
flushed with an empty (or suspiciously sparse) `Tags` map:

```go
if len(p.pending.Tags) == 0 {
    slog.Warn("flushing FULL_ENTITY block with empty Tags — indent regex may be wrong",
        "entity", p.pending.EntityName)
}
```

## Files to change

- `internal/parser/parser.go` — `reBlockTag` definition, flush warning

## Complexity

Low — regex change + warning log.

## Verification

- Unit test: feed a `FULL_ENTITY` block with 2-space and 8-space indented tags and
  confirm they are (or are not, depending on chosen option) captured.
- Confirm integration test still passes with 4-space indent in sample log.
