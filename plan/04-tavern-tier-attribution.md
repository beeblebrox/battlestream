# 04 — [BUG] `handleTagChange` may apply opponent's tier to local player

**Priority:** CRITICAL
**Status:** DONE
**Area:** `internal/gamestate/processor.go` — `handleTagChange`

## Problem

```go
if controllerID == p.localPlayerID || p.isLocalPlayerEntity(e) {
    p.machine.SetTavernTier(tier)
}
```

`isLocalPlayerEntity` falls back to name-string comparison. Early in a game, before
the local player name is known, `isLocalPlayerEntity` can return `false`. If `controllerID`
also resolves to `0` (entity not yet in registry), both guards fail, which is correct.

However, the dual-guard structure also means that if `controllerID == 0` AND
`isLocalPlayerEntity` returns `true` due to a name-match false-positive (see plan 08),
an opponent's tier change is applied to the local machine. The result is the TUI showing
the wrong tavern tier.

Additionally, there is no explicit check that `controllerID != 0` before trusting the
`controllerID == p.localPlayerID` branch — if `localPlayerID` is somehow also `0`
(unset at session start), every player's tier change would match.

## Fix

Add an explicit guard: only accept the tier change if:
1. `controllerID` is known (non-zero) AND matches `localPlayerID`, OR
2. The entity is positively identified as local via `localHeroID` (the entity-ID-based check),
   NOT via the name fallback.

```go
isLocal := (controllerID != 0 && controllerID == p.localPlayerID) ||
           (p.localHeroID != 0 && e.EntityID == p.localHeroID)
if isLocal {
    p.machine.SetTavernTier(tier)
}
```

Separately, ensure `localPlayerID` is initialized to a sentinel value (e.g., `-1`) rather
than `0` so that an unset ID can never accidentally match `controllerID == 0`.

## Files to change

- `internal/gamestate/processor.go` — `handleTagChange`, initialization of `localPlayerID`

## Complexity

Low — guard condition tightening.

## Resolution

Fixed: `handleTagChange` PLAYER_TECH_LEVEL/TAVERN_TIER case now requires
`controllerID != 0` before matching against `localPlayerID`. Falls back to
`isLocalPlayerEntity` only when controllerID is unknown (0). This prevents
`controllerID==0==localPlayerID` from matching when localPlayerID is unset.
