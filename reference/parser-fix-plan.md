# Parser Fix Plan

## Bugs Identified (2026-03-04)

### Bug 1: Armor/Health from wrong entities
**Symptom**: Armor shows 16 when it should be 7, fluctuates wildly.
**Cause**: ALL TAG_CHANGE events with HEALTH/ARMOR tags are applied to
`Player` regardless of which entity they belong to. Opponent hero health,
Bob's health (60), minion health — all overwrite the local player's stats.
**Fix**: Track the local player's CONTROLLER ID (PlayerID) and hero entity ID.
Only apply HEALTH/ARMOR from the hero entity owned by the local player.

### Bug 2: Turn number doubled
**Symptom**: Turn shows 5 when player is on turn 3.
**Cause**: Parser uses `TAG_CHANGE Entity=GameEntity tag=TURN value=N`.
In BG, GameEntity TURN counts both recruit AND combat phases (1=recruit,
2=combat, 3=recruit, etc.). The actual BG turn is half of this.
**Fix**: Use `TAG_CHANGE Entity=<localPlayerName> tag=TURN value=N` for
the display turn. Or divide GameEntity TURN by 2 and round up.

### Bug 3: Board mixes local and opponent minions
**Symptom**: Board shows wrong minions.
**Cause**: All FULL_ENTITY/SHOW_ENTITY with ATK/HEALTH create minions
in the single `Board` slice regardless of CONTROLLER.
**Fix**: Check entity's CONTROLLER/player field. Only add to Board if
controlled by the local player. Add to OpponentBoard otherwise.

### Bug 4: No log source filtering
**Symptom**: Events processed twice (once from GameState, once from PowerTaskList).
**Cause**: Parser processes all lines regardless of source.
**Fix**: Only parse lines from `GameState.DebugPrintPower()`, skip
`PowerTaskList.DebugPrintPower()`.

### Bug 5: Phase detection via modulo is wrong
**Symptom**: Phase may show COMBAT during recruit.
**Cause**: `SetTurn()` uses `turn%2` on the GameEntity turn to determine phase.
**Fix**: Use STEP tag changes (MAIN_READY, etc.) or track whose turn it is
based on whether the local player or dummy player is the CURRENT_PLAYER.

## Implementation Plan

### Step 1: Parser — filter log source
Only process lines containing `GameState.DebugPrintPower()`.
Skip `PowerTaskList` lines entirely.

### Step 2: Parser — extract player= from bracketed entities
Add regex to extract `player=N` from bracketed entity notation.
Include it in GameEvent so the processor can filter by controller.

### Step 3: Parser — capture Player entity definitions
Parse `Player EntityID=N PlayerID=M GameAccountId=[hi=X lo=Y]` lines.
Emit a new event type (EventPlayerDef) with this data.

### Step 4: Parser — capture PlayerName
Parse `GameState.DebugPrintGame() - PlayerID=N, PlayerName=XXX` lines.

### Step 5: Processor — identify local player
On EventPlayerDef, check GameAccountId. If hi≠0 and lo≠0, this is the
local player. Store their PlayerID as `localPlayerID`.

### Step 6: Processor — filter HEALTH/ARMOR by controller
For TAG_CHANGE with HEALTH/ARMOR, only apply to Player state if the
entity's player/controller matches localPlayerID.

### Step 7: Processor — use player TURN
Track turn from `TAG_CHANGE Entity=<localPlayerName> tag=TURN value=N`
instead of GameEntity TURN.

### Step 8: Processor — filter board by controller
For FULL_ENTITY with CONTROLLER, only add to Board if controller matches
localPlayerID. Add to OpponentBoard otherwise.

### Step 9: Processor — fix phase detection
Use STEP tag changes or current-player tracking instead of turn modulo.
