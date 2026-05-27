---
name: streamdeck-conventions
description: render.ts button layout, action extract() pattern, icon system for the StreamDeck plugin
metadata:
  type: project
---

## Button Layout (render.ts, 144x144px canvas)
- Three text zones stacked vertically:
  - Label (y=30): 17px bold, rgba(255,255,255,0.80) — e.g. "HEALTH", "GOLD"
  - Value (y=82): 52px bold, #ffffff — large primary stat
  - Subtitle (y=122): 14px normal, rgba(255,255,255,0.65) — secondary context
- Text shrinks via fitText() down to minSize (10px label, 14px value, 8px subtitle)
- Background: icon PNG with 0.45 black overlay (when online + icon exists), else gradient fallback
- Offline: gradient overrides to ['#2a2a2a', '#444444'] regardless of action gradient

## Icon System (post-restyle commit ad972ae, 2026-05-11)
- All icons: flat minimal, bold silhouette, black (#000000) background — see ICON-STYLE.md
- 30 PNGs in streamdeck-plugin/imgs/actions/
- Icon file name derived from action UUID suffix: e.g. `health` → `health.png`
- buff-slot.ts uses categoryIconPath() from categories.js for dynamic slot icons
- If icon load fails, falls back to gradient background silently

## Action Pattern (base.ts + action files)
- BaseStat abstract class: label + gradient (per-action) + extract(state) method
- extract() returns { value: string, subtitle: string }
- Offline sentinel: value='—', subtitle='OFFLINE' (rendered with dark gray gradient)
- Error sentinel: value='ERR', subtitle='' (catch in renderOne())
- Dedup: lastRenderKeys map prevents redundant re-renders (key = value|subtitle|offline)

## Known UX Issues (first review, 2026-05-23)
- Label zone at y=30 is only 30px from top — tight against the icon's visual center
- win-streak/loss-streak/triples/tavern-tier all have subtitle='' — no unit context
- health subtitle is "/ 30" — "/" prefix is unusual (convention is just "30" or "MAX: 30")
- armor subtitle='' — no "armor" unit; label says ARMOR but 0 armor looks same as no armor data
- win-streak label is "WIN STR." — abbreviation not obvious; "W STREAK" clearer
- loss-streak label "LOSS STR." same issue
- spell-power label "SPELL PWR" — fine but "SPELLS" shorter and equally clear
- tavern-wide-buff label "TVN WIDE" — "TVN" abbreviation non-obvious; "TAVERN" fits
- buff actions all use subtitle='' — could show total = atk+hp as subtitle e.g. "+12 total"
- win-streak shows 0 when no streak — no visual distinction from "never won"
- placement shows "—" during live game (pre-GAME_OVER) — correct but could say "LIVE" or "IN GAME"
- Dynamic buff-slot: empty slots render blank button (no label/value) — ambiguous to user

## Gradient Colors (per action)
- health: #7b0000 → #c0392b (dark to medium red)
- gold: #5c4a00 → #d4a017 (dark to gold)
- win-streak: #003366 → #2980b9 (dark to mid blue)
- loss-streak: #4a2000 → #e67e22 (dark brown to orange)
- bloodgem: #3a1a00 → #e67e22
- tavern-tier: #1a3a00 → #27ae60
- triples: #2d0060 → #8e44ad
- tavern-wide: #001a26 → #1a6b8a
Note: gradients only visible when icon fails to load (offline always uses gray; online uses icon+overlay)
