# 25 — [IMPROVEMENT] No historical board state query

**Priority:** LOW
**Area:** `internal/gamestate/`, `internal/api/`, `internal/store/`

## Problem

There is no way to ask "what was the board state at turn N?" All state is live and
mutable. Historical board snapshots are discarded after each combat round (only the
current board is kept). For debugging and replay purposes, this makes it impossible
to reconstruct the progression of the game.

## Fix

### Phase 1: Per-turn board snapshots in the store

At each turn boundary (recruit phase start), serialize the current board state and
store it as a sub-record keyed by `(gameID, turn)`:

```
store key: game/<gameID>/turn/<turnN>/board
value: JSON-encoded []Minion
```

### Phase 2: Query API endpoint

Add a REST endpoint:
```
GET /v1/game/<gameID>/turn/<turnN>/board
```

Returns the board state at the start of turn N.

Also add:
```
GET /v1/game/<gameID>/turns
```
Returns available turn numbers for a given game.

### Phase 3: TUI replay mode (optional, future)

A `battlestream replay <gameID>` command that steps through turns interactively.

## Files to change

- `internal/gamestate/processor.go` — serialize board at each turn boundary
- `internal/store/` — add per-turn board storage and retrieval
- `internal/api/rest/` — add turn/board endpoints
- Proto file — add `GetTurnBoard` RPC

## Complexity

High — new storage schema, new endpoints, new processor output. Phase 1 alone is medium.

## Verification

- After a full game, query `/v1/game/<id>/turn/3/board` and verify it matches the
  expected board from the sample log at turn 3.
