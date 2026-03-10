# 37 — Anomaly Display

**Priority:** MEDIUM
**Status:** TODO

## Problem

No anomaly info shown anywhere. During a Perfected Alchemy game, the anomaly
was active but invisible in TUI, REST, and gRPC outputs. Players need to know
which anomaly is in play since it fundamentally changes game strategy.

## Research Findings

From `power_log_2026_03_08b.txt`:
- **Entity ID:** 348
- **CardID:** `BG27_Anomaly_751` (Perfected Alchemy)
- **CARDTYPE:** `BATTLEGROUND_ANOMALY`
- **Created at:** Line 3237 (FULL_ENTITY block)
- **Zone:** PLAY
- **Owner:** Player 4 (local player)

Detection: Check for `CARDTYPE=BATTLEGROUND_ANOMALY` in FULL_ENTITY blocks.
Name resolution: `CardName("BG27_Anomaly_751")` in `cardnames.go`.

## Work

1. **Parse anomaly entity** — Detect FULL_ENTITY with
   `CARDTYPE=BATTLEGROUND_ANOMALY`. Store CardID and resolve name via
   `CardName()`.
2. **Store in BGGameState** — Add an `Anomaly` field (CardID + friendly name)
   to the game state struct.
3. **Display in TUI** — Show anomaly name in the TUI header/game info panel.
4. **Expose in APIs** — Add anomaly field to gRPC `GetCurrentGame` response
   and REST `/v1/game/current` endpoint.

## Test Data

- `internal/gamestate/testdata/power_log_2026_03_08b.txt` — Perfected Alchemy anomaly game

## Affected Files

- `internal/gamestate/state.go` — BGGameState struct (add Anomaly field)
- `internal/gamestate/processor.go` — parse anomaly entity
- `internal/tui/tui.go` — display anomaly in header
- `internal/api/grpc/server.go` — expose in gRPC response
- `internal/api/rest/handlers.go` — expose in REST response
- `proto/battlestream/v1/*.proto` — add anomaly to proto messages
