# 34 — Hero DAMAGE tag not tracked (health never updates)

**Priority:** CRITICAL
**Status:** DONE

## Problem

The processor handles `HEALTH` and `ARMOR` tag changes on the local hero entity
but ignores the `DAMAGE` tag entirely. In Battlegrounds, hero health works as:

    effective_health = HEALTH - DAMAGE

The `HEALTH` tag is the *base* (max) health and stays constant at 30 throughout
the game. The `DAMAGE` tag accumulates as the hero takes hits after armor is
depleted. Because DAMAGE is never processed, the TUI always shows health as 30.

Evidence from `power_log_2026_03_08.txt` (entity 101, Delivery Deryl):

| Event | HEALTH | ARMOR | DAMAGE | Effective HP |
|-------|--------|-------|--------|-------------|
| Initial | 30 | 16 | 0 | 30 |
| After turn 9 combat | 30 | 0 | 7 | 23 |
| After turn 11 combat | 30 | 0 | 17 | 13 |
| After turn 12 (death) | 30 | 0 | 32 | -2 |

The DAMAGE tag fires on the hero skin entity (localHeroID=101) in GameState
lines — it is not limited to combat-copy heroes.

## Root Cause

`processor.go` `handleTagChange()` has cases for `HEALTH` and `ARMOR` on the
local hero but no case for `DAMAGE`. The `applyTagToPlayer()` function in
`state.go` also has no DAMAGE handler.

## Fix

1. **Add `Damage` field to `PlayerState`** in `state.go`.
2. **Add `DAMAGE` case** in `processor.go` `handleTagChange()`:
   - When `isLocalHero(e, controllerID)`, call `machine.UpdatePlayerTag("DAMAGE", value)`.
3. **Add `DAMAGE` handler** in `applyTagToPlayer()` in `state.go`.
4. **Compute effective health** in the TUI and gRPC/REST API:
   - Display health as `Health - Damage` (effective), not raw `Health`.
   - Alternatively, store effective health directly in PlayerState.Health and
     keep MaxHealth separate (see plan 35).

## Affected Files

- `internal/gamestate/state.go` — PlayerState struct, applyTagToPlayer
- `internal/gamestate/processor.go` — handleTagChange DAMAGE case
- `internal/tui/tui.go` — health display calculation
- `proto/battlestream/v1/game.proto` — PlayerState message (add damage field)

## Test Data

`internal/gamestate/testdata/power_log_2026_03_08.txt` — full game with
video reference. Hero = Delivery Deryl (entity 101, player=7). Verify
effective health = 23 after turn 9, 13 after turn 11, dead (<=0) after turn 12.
