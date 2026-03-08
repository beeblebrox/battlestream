# 02 — [RISK] Parser state not reset between games

**Priority:** CRITICAL
**Status:** DONE
**Area:** `internal/parser/parser.go`

## Problem

`Parser.inBlock`, `Parser.pending`, and `Parser.blockStack` are never cleared when
`EventGameStart` is emitted. If the previous game ended mid-block (HS crash, truncated
log, daemon restart mid-game), the next game begins with stale block state.

The first non-block line of the new game will trigger `flushPending()`, emitting a garbage
`EventEntityUpdate` carrying leftover data from the previous game's unfinished block.
This can corrupt the new game's entity registry before any real events are processed.

## Impact

HIGH. Affects any session where the previous game did not terminate cleanly. Manifests as
phantom entities or wrong initial stats in the first turn of the following game.

## Fix

In the `EventGameStart` arm of `Feed()`, explicitly reset all block-tracking state before
emitting the event:

```go
case /* GameStart matched */:
    // Reset any leftover block state from the previous game
    p.inBlock = false
    p.pending = nil
    p.blockStack = p.blockStack[:0]
    p.out <- GameEvent{Type: EventGameStart, ...}
```

Alternatively, extract a `p.resetBlockState()` helper and call it from both `Feed()` and
any test setup paths.

## Files to change

- `internal/parser/parser.go` — add reset in `EventGameStart` arm of `Feed()`

## Complexity

Low — two or three lines in the right place.

## Resolution

Fixed: `parser.go` resets block state in two places when `reCreateGame` matches:
1. Inside the `if p.inBlock` check (line ~108): discards stale partial event.
2. In the main `switch` case for `reCreateGame` (line ~137): resets `inBlock`, `pending`, and `blockStack`.
