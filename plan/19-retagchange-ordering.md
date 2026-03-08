# 19 — [RISK] `reTagChange` catch-all has no documented priority over `reTurnStart`

**Priority:** LOW
**Status:** DONE
**Area:** `internal/parser/parser.go`

**Resolution:** Added block comment above the `reTurnStart` case documenting why it must
precede `reTagChange` — both match TAG_CHANGE lines but reTurnStart handles the specific
`tag=TURN` pattern.

## Problem

Both `reTurnStart` and `reTagChange` can match `TAG_CHANGE` lines. `reTurnStart` is
listed first in the switch, which is correct — it takes priority. However, this ordering
is undocumented and fragile. If a future log version adds a `TAG_CHANGE Entity=GameEntity
tag=STATE ...` line before the `TURN` line, it will fall through to the generic handler
and produce a spurious `EventTagChange` for `GameEntity` (mostly harmless noise, but
could confuse consumers).

## Fix

### Step 1: Document the ordering explicitly

Add a comment above the switch cases:

```go
// IMPORTANT: reTurnStart must appear before reTagChange. Both match TAG_CHANGE lines,
// but reTurnStart checks for the specific TURN tag on GameEntity. The catch-all
// reTagChange must come last to avoid consuming lines that more specific patterns
// should own.
```

### Step 2: Add a test

Write a unit test that feeds a `TAG_CHANGE Entity=GameEntity tag=TURN value=3` line
and asserts:
- An `EventTurnStart` is emitted, not `EventTagChange`.
- No duplicate events are produced.

This documents the expected ordering in an executable form.

### Step 3: Consider a dedicated match for GameEntity STATE changes (optional)

If Blizzard ever adds new GameEntity-scoped `TAG_CHANGE` lines that should be handled
specifically, add them before `reTagChange` in the switch.

## Files to change

- `internal/parser/parser.go` — add ordering comment
- `internal/parser/parser_test.go` — add ordering unit test

## Complexity

Very low — comments and a test; no logic changes.

## Verification

- New unit test passes.
- Integration test still passes.
