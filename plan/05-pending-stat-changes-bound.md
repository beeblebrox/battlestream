# 05 — [RISK] `pendingStatChanges` is never bounded

**Priority:** HIGH
**Status:** DONE
**Area:** `internal/gamestate/processor.go`

## Problem

`pendingStatChanges` accumulates stat-change events and is flushed at turn boundaries.
If a turn boundary event is missed (parser edge case, truncated log line, unexpected
log format), the buffer is never flushed. On the next turn, the old changes are still
present and will be grouped with new changes, producing incorrect cross-turn `Modifications`
matches — buffs attributed to the wrong turn, or false board-wide buff detections.

In theory, flush happens at every turn boundary and is bounded by minions-per-turn.
In practice, one missed boundary event causes silent unbounded accumulation.

## Fix

### Option A: Cap and flush-on-capacity

Add a per-turn capacity constant (e.g., `maxPendingStatChanges = 200`). After appending
to `pendingStatChanges`, check length and flush if exceeded:

```go
p.pendingStatChanges = append(p.pendingStatChanges, sc)
if len(p.pendingStatChanges) > maxPendingStatChanges {
    slog.Warn("pendingStatChanges cap reached, flushing early", "count", len(p.pendingStatChanges))
    p.flushPendingStatChanges()
}
```

### Option B: Flush on EventGameStart as well

Ensure `flushPendingStatChanges()` is also called when a new game starts, preventing
inter-game leakage as a secondary safeguard.

Both options should be applied.

## Files to change

- `internal/gamestate/processor.go` — add cap check after append; add flush in GameStart handler

## Complexity

Low — a few lines of guard code.

## Resolution

Fixed: Both options implemented.
- `maxPendingStatChanges = 200` constant defined and enforced after each append with a warning log.
- `flushPendingStatChanges()` called on `EventGameStart` and `EventGameEnd` in the Handle switch.
