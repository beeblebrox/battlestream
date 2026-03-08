# 15 — [BUG] `EventPlayerUpdate` and `EventZoneChange` declared but never emitted

**Priority:** MEDIUM
**Area:** `internal/parser/events.go`, `internal/parser/parser.go`

## Problem

`events.go` declares `EventPlayerUpdate` and `EventZoneChange` but the parser never
emits them. Zone changes arrive as `EventTagChange` with `tag=ZONE`, handled correctly
by the processor. However, consumers (REST/gRPC clients, external tools) that register
handlers for these event types receive nothing — a silent API contract violation.

## Options

### Option A: Remove the dead constants (preferred if no external consumers)

Delete `EventPlayerUpdate` and `EventZoneChange` from `events.go`. Update any switch
statements or handler maps that reference them to remove those dead cases.

This is the cleanest fix if these event types were never part of a public API contract.

### Option B: Emit the events explicitly

Route zone changes through `EventZoneChange`:
- In the processor's `handleTagChange`, when `tag == "ZONE"`, emit an `EventZoneChange`
  in addition to (or instead of) the generic `EventTagChange`.

Route player updates through `EventPlayerUpdate`:
- When any player-scoped tag changes (health, tier, gold), emit `EventPlayerUpdate`.

Option B is more work and adds event volume. Only pursue if external consumers are
documented to depend on these types.

### Option C: Deprecate with a compile-time comment

If removing would break external tools, mark the constants as deprecated:
```go
// Deprecated: never emitted by the parser. Use EventTagChange with tag=ZONE instead.
EventZoneChange EventType = "zone_change"
```

## Recommendation

Audit external consumers first. If none depend on these event types, remove them (Option A).
If the intent was always to emit them, implement Option B for completeness.

## Files to change

- `internal/parser/events.go` — remove or deprecate constants
- Any switch/map referencing these constants in processor, API handlers, or tests

## Complexity

Low — mechanical removal or targeted addition.

## Verification

- Grep for all references to `EventPlayerUpdate` and `EventZoneChange`; confirm all are
  removed or handled after the change.
- Integration test should still pass.
