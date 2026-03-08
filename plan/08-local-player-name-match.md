# 08 — [BUG] `isLocalPlayerEntity` name-string match can false-positive

**Priority:** HIGH
**Status:** DONE
**Area:** `internal/gamestate/processor.go` — `isLocalPlayerEntity`

## Problem

```go
if p.localPlayerName != "" && e.EntityName == p.localPlayerName {
    return true
}
```

If another player in the lobby has the same BattleTag display name as the local player
(possible via name changes, edge cases, or in custom/bot lobbies), their tag changes
will be attributed to the local player. This propagates wrong tavern tier, triples count,
or other player-scoped state into the machine.

The entity-ID-based check (`localHeroID`) is more reliable. The name check is a fragile
last resort.

## Fix

### Option A: Demote name check to last resort with a warning

```go
func (p *Processor) isLocalPlayerEntity(e TagChangeEvent) bool {
    if p.localHeroID != 0 && e.EntityID == p.localHeroID {
        return true
    }
    if p.localPlayerID != 0 {
        return false // we have a positive ID, don't fall back to name
    }
    // last resort: name match, but log a warning
    if p.localPlayerName != "" && e.EntityName == p.localPlayerName {
        slog.Warn("isLocalPlayerEntity: using name fallback — localHeroID not set",
            "name", p.localPlayerName, "entityID", e.EntityID)
        return true
    }
    return false
}
```

### Option B: Remove name fallback entirely

Once `localHeroID` is reliably populated from `GameAccountId=[hi=<nonzero>]` during
`CREATE_GAME`, the name check is unnecessary. Audit whether there is any scenario where
`localHeroID` is not set but `localPlayerName` is. If not, remove the name branch.

Option A is safer as a first step; Option B once confirmed by testing.

## Files to change

- `internal/gamestate/processor.go` — `isLocalPlayerEntity`

## Complexity

Low — guard condition change + optional warning log.

## Resolution

Fixed with Option A (demoted name to last resort with warning):
- Prefers `PlayerID` match (most reliable).
- Only uses name fallback when `localPlayerID == 0` (with `slog.Warn`).
- Additional safe bare-name fallback for events with `e.PlayerID == 0` when localPlayerID is known.
