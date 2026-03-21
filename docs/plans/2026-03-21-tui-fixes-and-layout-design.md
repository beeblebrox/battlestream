# TUI Bug Fixes and Layout Overhaul

**Date:** 2026-03-21
**Approach:** Fix-first, then enhance (Approach A)

## Issues

1. **TUI crash at partner tier 7** — instant crash when duo partner tiers to 7 via anomaly; cobra prints help text (no `SilenceUsage`)
2. **Opponent buff leakage** — opponent beetle/blood gem buffs counted in local BUFF SOURCES during combat
3. **Partner buff leakage** — partner buffs also counted in local BUFF SOURCES during combat
4. **Partner pane scroll broken** — mouse wheel and scrollbar not routed to partner viewport
5. **First-game keys/streak broken** — shortcuts and streak display didn't work until restart; tab (CombinedModel level) still worked
6. **Auto-fill layout** — panes should fill available space after meeting minimums
7. **Mouse-drag pane resizing** — vertical + horizontal dividers draggable, sizes persisted to config

## Phase 1: Bug Fixes

### 1.1 Cobra SilenceUsage

Add `SilenceUsage: true` and `SilenceErrors: true` to root cobra command in `cmd/battlestream/main.go`. Prevents cobra from printing help/usage when `RunE` returns an error.

### 1.2 Tier 7+ Rendering

Fix `renderTavernTier()` in `tui.go`:
- Change `6-tier` to `max(0, 6-tier)` for empty stars
- Add tier 7 color case for anomaly tier

### 1.3 Buff Attribution Fix + Partner Buff Tracking

**Root cause:** During combat, opponent/partner enchantment entities can have TAG_CHANGE events processed before their CONTROLLER is registered in `entityController`. The TAG_SCRIPT_DATA guard at processor.go line 692-696 and the enchantment relevance filter at line 1469-1483 have gaps that allow non-local enchantments through.

**Fix:**
- Add combat-phase guard in TAG_SCRIPT_DATA handling: if phase is COMBAT and entity is not in a known-local set, reject
- Create `partnerBuffs buffTracker` alongside existing `localBuffs buffTracker` in processor
- When enchantment controller matches partner player ID, route to `partnerBuffs`
- Expose `PartnerBuffSources` and `PartnerAbilityCounters` on `BGGameState`
- Unreserve proto fields 20-21 in game.proto for partner buff data
- Build regression tests using `lastgames/` Power.log + PartnerMacPlayer.log

### 1.4 Partner Pane Scroll

In `tui.go`:
- Add `partnerBoardVP` to mouse wheel routing in `handleMouse()` — check Y position against `partnerVPY`/`partnerVPH`
- Add partner scrollbar to `identifyScrollbar()`
- Add partner panel to `scrubAt()` routing

### 1.5 First-Game Key/Streak Bug

Defensive fixes:
- Add `slog.Debug` logging in `CombinedModel.Update()` for key forwarding
- Audit `switchMode()` and init path to ensure `c.live` is never re-created
- Verify no path exists where `m.game` is populated but keys are swallowed
- Regression test: simulate startup sequence and verify key state propagation

## Phase 2: New Features

### 2.1 Partner Buff Sources Pane

New viewport panel displaying partner buff data captured from combat enchantments.

Layout (Duos only):
```
┌─────────────────────┬───────────────────────┐
│ BOARD               │ BUFF SOURCES          │
│ local minions       │ local buffs           │
│                     │                       │
├─────────────────────┼───────────────────────┤
│ PARTNER BOARD       │ PARTNER BUFFS (last)  │
│ partner minions     │ partner buffs         │
│                     │                       │
└─────────────────────┴───────────────────────┘
```

Partner buff data arrives during combat (from combat copy enchantments):
- Updated once per combat round, not live during recruit
- Labeled "last seen" with stale indicator similar to partner board

### 2.2 Proportional Layout Engine

Replace fixed `colW := m.width/2 - 4` with ratio-based system:
- **Vertical split ratio** (default 0.5): left/right column widths
- **Horizontal split ratio** (default 0.7): row 1 vs row 2 height (Duos only)
- Minimum sizes preserved (10 chars wide, 4 lines tall)
- Remaining space fills proportionally

### 2.3 Mouse-Drag Dividers

Two draggable boundaries:
1. **Vertical divider** — `│` column between left/right panels. Both rows share this divider.
2. **Horizontal divider** — `├───┤` row between main and partner panels (Duos only).

Detection on `MouseActionPress`, update ratio on `MouseActionMotion`, persist on `MouseActionRelease`.

### 2.4 Persisted Split Ratios

Add to `~/.battlestream/config.yaml`:
```yaml
tui:
  vertical_split: 0.5
  horizontal_split: 0.7
```

- Loaded on TUI startup, written on drag release
- Clamped when terminal is too small for saved ratio, but saved value preserved

## Test Data

- `lastgames/Hearthstone_2026_03_20_20_10_45/Power.log` — 4 duos games, tier 7 anomaly, beetle buff leakage
- `lastgames/PartnerMacPlayer.log` — same session from partner's perspective
- `~/.battlestream/battlestream.log` — daemon log showing 224K event drops, no panic

## Execution Order

Sequential within each phase. Skip `-race` during iterative testing; only use `-race -count=1` on final full validation.

1. SilenceUsage fix
2. Tier 7 rendering fix
3. Buff attribution fix + partner buff tracker + proto regen
4. Partner pane scroll fix
5. First-game key/streak investigation
6. Partner buff sources pane in TUI
7. Proportional layout engine
8. Mouse-drag dividers
9. Persist split ratios to config
