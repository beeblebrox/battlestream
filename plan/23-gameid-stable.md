# 23 — [IMPROVEMENT] `gameSeq` not stable across restarts

**Priority:** LOW
**Area:** `internal/gamestate/`, daemon startup

## Problem

`gameSeq` is an in-process counter, reset to 0 on daemon restart. If the daemon restarts
mid-session, the next game gets `game-1` again, which will collide with a previously
stored game if the store key includes this ID. The BadgerDB store uses a separate dedup
key, so collisions are partially handled, but the collision still appears in logs and
may confuse tooling or external clients that use game IDs as stable references.

## Fix

Derive game ID from the session start timestamp (first `CREATE_GAME` timestamp):

```go
// In processor, on GameStart:
gameID := fmt.Sprintf("game-%d", sessionStartTime.UnixMilli())
```

This is stable across daemon restarts because the `CREATE_GAME` timestamp is embedded
in the log file and does not change. On reparse, the same game will produce the same ID.

### Additional consideration

If `extractTimestamp` is still using today's date (see plan 16), the game ID will still
be wrong for reparsed old logs. Plans 16 and 23 should ideally be fixed together.

## Files to change

- `internal/gamestate/processor.go` — derive `gameID` from `CREATE_GAME` timestamp
- `internal/store/` — update any store key format that uses `gameSeq` directly
- `internal/gamestate/machine.go` — update `GameID` field population

## Complexity

Low-medium. Depends on plan 16 for full correctness but can be implemented independently.

## Verification

- Restart the daemon mid-session and confirm the resumed game gets the same ID as
  before the restart.
- Reparse a log and confirm game IDs match those from the live parse.
