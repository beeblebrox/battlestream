# 29 — [IMPROVEMENT] WS/SSE broadcast full state on every event

**Priority:** LOW
**Area:** `internal/api/rest/` — SSE/WebSocket handlers

## Problem

The current SSE/WebSocket implementation broadcasts the complete `BGGameState` snapshot
on every event. During high-frequency log phases (e.g., combat with many TAG_CHANGE
events), this can produce dozens of full-state pushes per second, each serializing the
entire board, enchantments, modifications, and buff sources. This wastes bandwidth and
CPU for clients that only care about specific fields.

## Fix

### Option A: Minimum emit interval (throttle)

Coalesce events within a configurable window (e.g., 50ms) and emit only the latest
snapshot. This is the simplest approach:

```go
const sseThrottle = 50 * time.Millisecond

// In SSE handler goroutine:
ticker := time.NewTicker(sseThrottle)
for range ticker.C {
    state := machine.State()
    if state.Generation != lastGen {
        emit(state)
        lastGen = state.Generation
    }
}
```

Requires plan 27 (generation counter) to detect no-change efficiently.

### Option B: Delta/patch protocol

Compute a JSON Merge Patch (RFC 7396) or JSON Patch (RFC 6902) between the previous
and current state. Send only the diff. Clients apply the patch to their local copy.

This is more complex to implement and requires clients to maintain local state.
Only worthwhile if bandwidth is a genuine constraint (e.g., remote streaming over WAN).

### Option C: Field-scoped subscriptions

Allow SSE clients to subscribe to specific fields:
```
GET /v1/events?fields=board,player.health
```

Server only serializes and sends those fields. More flexible but significantly more
implementation complexity.

**Recommendation:** Option A (throttle at 50ms) is a quick win with minimal risk.
Option B if bandwidth becomes a real constraint.

## Files to change

- `internal/api/rest/` — SSE and WebSocket handlers
- Requires plan 27 (generation counter) for efficient change detection

## Complexity

Low (Option A) / High (Option B or C).

## Verification

- Under combat-phase log replay, confirm SSE emits at most 20 events/sec (1/50ms)
  rather than hundreds.
- Client receives final correct state at the end of combat.
