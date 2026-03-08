# 10 — [IMPROVEMENT] No opponent tracking

**Priority:** MEDIUM
**Area:** `internal/gamestate/`, `internal/tui/`, `internal/api/`

## Problem

`BGGameState.Opponent` and `OpponentBoard` exist in the struct but are never populated.
Opponent hero, health, board, and buffs are not tracked. This is a major feature gap
for overlay/stream use cases that want to show who you're fighting each combat round.

## What HS logs provide

- `TAG_CHANGE Entity=<opponent hero> tag=HEALTH value=<n>` during combat
- FULL_ENTITY / SHOW_ENTITY blocks for opponent minions (they exist on the board for combat)
- `TAG_CHANGE ... tag=ZONE value=PLAY` for opponent minions entering the board
- `TAG_CHANGE Entity=GameEntity tag=NEXT_OPPONENT_PLAYER_ID value=<n>` (matchup preview)

## Fix plan

### Phase 1: Identify opponent player each turn

During recruit phase, `NEXT_OPPONENT_PLAYER_ID` tag change on `GameEntity` provides the
upcoming opponent's player ID. Store this as `p.opponentPlayerID`.

### Phase 2: Track opponent hero entity

Identify the opponent's hero entity ID from the entity registry (similar to how
`localHeroID` is found). Store as `p.opponentHeroID`.

### Phase 3: Populate OpponentBoard during combat

During `PhaseCombat`, track ZONE=PLAY transitions for entities controlled by
`opponentPlayerID`. Add them to `machine.OpponentBoard` (or equivalent).
Clear opponent board at combat end (when entities return to SETASIDE/GRAVEYARD).

### Phase 4: Populate Opponent health and name

From entity registry and HEALTH tag changes for opponent hero.

### Phase 5: Expose via API

`BGGameState.Opponent` should include: name, health, tavern tier, current board.
Wire through REST/gRPC and TUI.

## Files to change

- `internal/gamestate/processor.go` — opponent tracking logic
- `internal/gamestate/machine.go` — store and expose opponent state
- `internal/tui/` — render opponent panel
- `internal/api/rest/` + `internal/api/grpc/` — expose in response types
- Proto file — add opponent fields if not already present

## Complexity

High — significant new feature. Multiple phases, each testable independently.
Phase 1-2 (identification) is low complexity. Phase 3+ is medium-high.

## Verification

- Unit test: opponent player ID correctly extracted from `NEXT_OPPONENT_PLAYER_ID` event.
- Integration test: opponent board populated during combat rounds (requires log with
  visible opponent minions).
