# 35 — Max health hardcoded to 40 (should be 30 from hero HEALTH tag)

**Priority:** HIGH
**Status:** DONE

## Problem

The TUI health bar uses `maxHP := int32(40)` (tui.go:442). In Battlegrounds,
all heroes have 30 base health (HEALTH=30 on the hero entity). The bar shows
"30/40" which is wrong — it should be "30/30" at full health.

The max health value is available from the hero's initial HEALTH tag in the
CREATE_GAME block (e.g., `FULL_ENTITY Creating ID=37 ... tag=HEALTH value=30`)
and is later confirmed when the hero skin entity is assigned.

## Fix

1. **Add `MaxHealth` field to `PlayerState`** in `state.go`. Default to 30
   (current BG standard) but set from actual tag data.
2. **Set MaxHealth from hero HEALTH tag** — when the hero entity is first
   identified (`handlePlayerDef` / `handleEntityUpdate`), capture the HEALTH
   tag value as MaxHealth.
3. **Expose MaxHealth in proto** — add `max_health` to `PlayerState` message.
4. **Use MaxHealth in TUI** — replace `maxHP := int32(40)` with
   `maxHP := p.MaxHealth` (with fallback to 30 if 0).

## Affected Files

- `internal/gamestate/state.go` — PlayerState struct, GameStart default
- `internal/gamestate/processor.go` — set MaxHealth when hero identified
- `internal/tui/tui.go` — use proto MaxHealth instead of hardcoded 40
- `proto/battlestream/v1/game.proto` — add max_health to PlayerState

## Interaction with Plan 34

Plan 34 (DAMAGE tracking) and this plan together fix the health display:
- Effective health = MaxHealth - Damage (when HEALTH tag = MaxHealth)
- Health bar: `renderHealthBar(effectiveHP, maxHP, 16)`
