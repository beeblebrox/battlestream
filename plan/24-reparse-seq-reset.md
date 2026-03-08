# 24 — [IMPROVEMENT] Reparse does not reset `gameSeq`

**Priority:** LOW
**Status:** DONE (moot — resolved by plan 23)
**Area:** `cmd/battlestream/` — reparse subcommand, `internal/gamestate/`

**Resolution:** Timestamp-based game IDs (plan 23) eliminate the counter dependency.
Same log produces same game ID regardless of daemon/reparse context.

## Problem

`battlestream reparse` re-feeds all historical log lines through a fresh parser and
processor but the daemon's in-process `gameSeq` counter is not reset first. If the
daemon is running while reparse is triggered, seq numbers will be inconsistent between
the daemon's live state and the reparsed store entries.

This is a secondary issue that becomes moot if plan 23 (stable timestamp-based game IDs)
is implemented.

## Fix

### Option A: Implement plan 23 first

Timestamp-based game IDs eliminate the counter entirely, so this issue disappears.

### Option B: Reset counter in reparse command

If `gameSeq` is kept, ensure the reparse command creates a fresh processor with `gameSeq`
reset to 0 and a note in the store that these are reparsed entries:

```go
// In reparse subcommand:
processor := gamestate.NewProcessor(store, /* gameSeq = 0 */)
```

The live daemon should not share the same counter with the reparse process.

### Option C: Isolate reparse to a separate processor instance

Reparse should always create a fully independent processor instance, not reuse the
daemon's processor. This should already be the case if reparse runs as a separate
command, but verify there is no shared mutable state.

## Files to change

- `cmd/battlestream/` — reparse subcommand
- `internal/gamestate/` — verify processor isolation

## Complexity

Low (if plan 23 is done first — this becomes a no-op).

## Verification

- Run daemon, then `battlestream reparse`, then check store entries — game IDs should
  be consistent.
