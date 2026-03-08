# 06 — [RISK] Board snapshot/restore unconditional on non-empty

**Priority:** HIGH
**Status:** DONE
**Area:** `internal/gamestate/processor.go` — `GameEnd` handler, `UpdateBoardSnapshot`

## Problem

`GameEnd` unconditionally restores `boardSnapshot` if it is non-empty, even if a valid
recruit-phase board was already established after the last combat round. If
`UpdateBoardSnapshot` was called during the final combat (via `tryAddMinionFromRegistry`
for combat copy entities), the snapshot may contain combat copies with base (un-buffed)
stats rather than the buffed stats from the recruit phase.

Result: the game-over board displayed/stored may show lower ATK/HEALTH than the player
actually had.

## Investigation needed

Before coding the fix, verify with the integration test log exactly when
`UpdateBoardSnapshot` is called during the final combat round:

1. Is it called for combat copies (SETASIDE entities) before their TAG_CHANGE buff
   sequence completes?
2. Or only after the full TAG_CHANGE sequence?

If (1), the snapshot may be overwritten with base stats before buffs are applied, and
the restore at GameEnd uses those base stats.

## Fix

### Option A: Snapshot phase gate

Only call `UpdateBoardSnapshot` during recruit phase (`PhaseRecruit`), not during
`PhaseCombat`. The final recruit-phase snapshot is then always the correct buffed board.

```go
func (p *Processor) tryAddMinionFromRegistry(entityID int, zone string) {
    if p.phase == PhaseCombat {
        return // don't overwrite recruit snapshot with combat copies
    }
    // ... existing logic
}
```

### Option B: Two-snapshot model

Maintain separate `recruitSnapshot` and `combatSnapshot`. `GameEnd` always restores
from `recruitSnapshot`. `combatSnapshot` is used only for combat-phase display.

Option A is simpler and sufficient.

## Files to change

- `internal/gamestate/processor.go` — gate `UpdateBoardSnapshot` on `PhaseRecruit`

## Complexity

Low — one guard condition, but requires careful verification with the integration test
log to confirm the phase at the time `UpdateBoardSnapshot` is called.

## Resolution

Fixed with Option A (snapshot phase gate): `tryAddMinionFromRegistry` only calls
`UpdateBoardSnapshot()` when `p.machine.Phase() == PhaseRecruit`. Combat copies
arriving during PhaseCombat do not overwrite the recruit board snapshot.
