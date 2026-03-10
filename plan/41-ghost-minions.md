# 41 — Ghost Minions on Board

**Priority:** HIGH
**Status:** TODO

## Problem

Embalming Expert and extra Eternal Night showing on board when not actually in
play. Board count reached 9 (max is 7 in standard BG). This indicates zone
transition handling is broken — minions that were sold, tripled, or removed
are not being cleaned up from the board state.

## Research Findings

**Embalming Expert (entity 1577, CardID BG34_691):**
- Created at line 18652 with CONTROLLER=12 (opponent), ZONE=PLAY
- No subsequent ZONE tag changes — stays in PLAY throughout
- Should NOT appear on local player board (localPlayerID=4)

**Processor guards appear correct:**
- `handleEntityUpdate` (line 604-613): checks `controllerID == p.localPlayerID`
- Zone transitions (line 305-327): check controller before removal/addition
- GAME_OVER guard blocks removal to preserve final board

**Possible causes (narrowed):**
- Controller check may fail for entities where PlayerID isn't set on TAG_CHANGE events
- `resolveController` fallback may return wrong value for some entity transitions
- Combat copy creation may bypass controller checks

## Work

1. **Reproduce with test data** — Parse `power_log_2026_03_08b.txt` and
   identify turns where board count exceeds 7.
2. **Trace zone transitions** — For the ghost entities (Embalming Expert,
   extra Eternal Night), trace their full lifecycle in the log to find which
   ZONE transition was missed.
3. **Fix processor** — Update zone transition handling in `processor.go` to
   correctly remove minions in all cases.
4. **Add regression test** — Verify board never exceeds 7 minions for this
   game.

## Test Data

- `internal/gamestate/testdata/power_log_2026_03_08b.txt`
  - Turn 8: Embalming Expert ghost
  - Last turn: Extra Eternal Night + 9-card board count

## Related

- Issues #5, #6, #8 — same root cause (ghost minions)
- `plan/06-board-snapshot-restore.md` — board snapshot/restore logic

## Affected Files

- `internal/gamestate/processor.go` — zone transition handling
- `internal/gamestate/processor_test.go` — regression test
