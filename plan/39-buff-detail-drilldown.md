# 39 — Buff Detail Drill-down

**Priority:** LOW
**Status:** TODO

## Problem

No way to see individual buff events — only aggregate counts per source.
Users want to understand when buffs were applied, what the per-turn values
were, and how their buff sources scaled over the course of the game.

## Work

1. **Track buff event history** — Store per-turn buff applications in the
   game state (source, turn, ATK delta, HP delta).
2. **TUI drill-down** — Add interactive selection in the BUFF SOURCES panel.
   Selecting a buff source opens a detail popup showing per-turn history
   with values.
3. **API exposure** — Add buff detail list to gRPC and REST responses so
   external tools can access the history.

## Test Data

- `internal/gamestate/testdata/power_log_2026_03_08b.txt`

## Affected Files

- `internal/gamestate/state.go` — add BuffEvent history to BGGameState
- `internal/gamestate/processor.go` — record individual buff events
- `internal/tui/tui.go` — interactive drill-down UI
- `internal/api/grpc/server.go` — expose buff detail in gRPC
- `internal/api/rest/handlers.go` — expose buff detail in REST
- `proto/battlestream/v1/*.proto` — add buff event messages
