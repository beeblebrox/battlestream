# 40 — Gold Tracking Display

**Priority:** MEDIUM
**Status:** TODO

## Problem

Max gold not shown anywhere. User had 15 gold on turn 8 (above normal 10-gold
cap due to Perfected Alchemy anomaly) but this was invisible in the TUI and
APIs.

## Research Findings

From `power_log_2026_03_08b.txt`:
- **Tag names:** `RESOURCES` (total gold), `RESOURCES_USED` (spent gold)
- **Entity:** Player entity (Moch#1358), NOT the hero entity
- **Initial gold:** 3 (line 2118)
- **Turn 8 gold:** 15 (line 73660) — above 10-gold cap due to Perfected Alchemy
- **Turn tag:** Moch#1358 TURN=8 at line 73637, then RESOURCES=15 at line 73660

Available gold = RESOURCES - RESOURCES_USED.

## Work

1. **Check existing parsing** — `plan/13-gold-tracking.md` may already cover
   the parsing side. Track `RESOURCES` and `RESOURCES_USED` player tags.
2. **Wire into TUI** — Display current/max gold in the game info panel
   (e.g. "Gold: 7/15").
3. **Wire into APIs** — Expose gold info in gRPC and REST responses.

## Test Data

- `internal/gamestate/testdata/power_log_2026_03_08b.txt` (turn 8 — 15 gold)

## Related

- `plan/13-gold-tracking.md` — original gold tracking plan

## Affected Files

- `internal/gamestate/state.go` — add gold fields to BGGameState (if not present)
- `internal/gamestate/processor.go` — track RESOURCES/RESOURCES_USED tags
- `internal/tui/tui.go` — display gold in game info panel
- `internal/api/grpc/server.go` — expose in gRPC
- `internal/api/rest/handlers.go` — expose in REST
