# Stats Viewer Web Dashboard — Design

**Date:** 2026-03-22
**Branch:** stats-viewer
**Status:** Approved

## Overview

A web-based stats dashboard for Hearthstone Battlegrounds, served from the existing daemon REST server. Provides session trends, comparative solo/duos analysis, partner breakdowns, and per-game drill-down — all visualized with Apache ECharts.

## Technology

- **Charting:** Apache ECharts (dark theme) — 65K GitHub stars, Apache 2.0 license, automatic animations, 20+ chart types including native heatmaps
- **Embedding:** Go `embed.FS` packages HTML/JS/CSS into the binary. No build tooling, no npm, no separate process.
- **Serving:** Existing REST server on `:8080` serves dashboard at `/dashboard`
- **Data:** Fetched from existing REST endpoints plus minimal new additions

## API

### Existing endpoints (no changes)

- `GET /v1/stats/aggregate` — overall aggregate stats
- `GET /v1/stats/games?limit=N&offset=N` — paginated game list (GameMeta: ID, timestamps, placement, is_duos). Returns `total` count.
- `GET /v1/game/{id}` — full game state (hero, board, buffs, anomaly, partner, etc.)

### New endpoints

- `GET /v1/game/{id}/turns` — turn snapshots for per-game drill-down. Backed by existing `store.GetTurnSnapshots()`.
- `GET /v1/stats/games?mode=solo|duos|all` — mode filter parameter on game list
- `GET /v1/stats/aggregate?mode=solo|duos|all` — mode filter parameter on aggregates

No proto/gRPC changes — these are REST-only since the dashboard is the sole consumer.

### Pagination strategy

`ListGames` already supports `limit`, `offset`, and returns `total`. GameMeta is small (~100 bytes each), so for trend charts the dashboard fetches all metas in pages client-side. Full game state is only fetched on drill-down into a single game.

## Dashboard Layout

### Three-level drill-down flow

```
Level 1 (Session Overview)
  → click data point representing a game
Level 2 (Single Game — turn-by-turn)
  → click a turn
Level 3 (Turn Detail — board snapshot)
```

### Level 1 — Session Overview (landing page)

#### Mode toggle: Solo / Duos / Compare

- **Solo** — filters to solo games only
- **Duos** — filters to duos games, adds partner sub-tabs
- **Compare** — overlays solo and duos on the same charts with separate trend lines and delta indicators

#### Duos sub-tabs: My Stats / Partner Stats / By Partner

- **My Stats** — your personal stats filtered to duos games
- **Partner Stats** — aggregate partner hero performance, partner tavern tier, partner board stats (from combat snapshots)
- **By Partner** — multi-select partner filter with:
  - Checkboxes for each battletag
  - Quick actions: All, None, Invert
  - Click a name to solo-select; shift-click or checkboxes for multi-select
  - Exclude toggle per partner (easy "all but one" filtering)
  - All charts react to the current partner selection
  - Table: partner battletag, games played, avg placement, win rate

#### Summary cards row

Games Played, Win Rate, Avg Placement, Best/Worst, Current Streak. In Compare mode, shows both with delta indicators.

#### Charts

| Chart | Type | Notes |
|-------|------|-------|
| Placement over time | Line + trend line | Solo+Duos overlaid in Compare |
| Placement distribution | Bar (1st–8th) | Side-by-side in Compare |
| Win rate trend | Rolling average line | Configurable window |
| Hero performance | Horizontal bar (avg placement) | Sortable |
| Tavern tier reached | Distribution bar | Max tier per game |
| Buff source breakdown | Stacked bar/pie | Aggregate across games |
| Game duration vs placement | Scatter | Duration buckets |
| Anomaly performance | Bar (avg placement per anomaly) | |
| Tribe/comp win rates | Grouped bar | Dominant tribe on final board |
| Buff source efficiency | Scatter (total buffs vs placement) | Unique to battlestream |
| Session streaks | Timeline | Streak highlights |
| Minion play impact | Table | Most-used final-board minions + avg placement |

#### Heatmaps (4)

1. **Hero × Placement** — frequency of each placement per hero. Answers: "which heroes do I consistently top-4 with?"
2. **Tavern Tier × Turn Reached** — game count at each tier/turn intersection. Answers: "am I leveling at the right tempo?"
3. **Tribe × Placement** — frequency of each placement per dominant tribe. Answers: "which comps do I pilot best?"
4. **Buff Amount × Placement** — game count at each buff-total/placement intersection. Answers: "do bigger buffs correlate with better finishes?" (unique to battlestream)

All heatmaps respect the mode toggle and partner filter.

### Level 2 — Single Game Overview

#### Header

Hero name, placement badge, date, duration, anomaly, solo/duos badge. In duos: partner battletag and hero.

#### Turn-by-turn charts (shared x-axis: turn number)

| Chart | Type |
|-------|------|
| Board total ATK/HP | Dual-axis line |
| Health/armor trajectory | Line |
| Tavern tier progression | Step chart |
| Buff accumulation by category | Stacked area |
| Gold economy (total, used) | Line |
| Board size (minion count) | Line |

### Level 3 — Turn Detail

- Board snapshot: minion cards with ATK/HP, enchantments
- Buff deltas for that turn (category, ATK/HP change)
- Ability deltas
- Modifications applied

## Theme

ECharts built-in dark theme. Gaming aesthetic with smooth animated transitions between drill-down levels.

## Out of Scope (follow-up)

- **Buy/sell tracking** — card transaction history with tribe/gold-type breakdowns. Requires new parser/processor work to capture `TB_BaconShop_DragBuy`/`TB_BaconShop_DragSell` events.
- **MMR/rating tracking** — not available from Power.log
- **Opponent warband history** — partial data available, complex to implement
- **Combat per-round win/loss timeline** — requires inference from damage deltas

## Data Available per Game (from Power.log)

- Player: name, hero CardID, health, armor, tavern tier, gold, triples, damage, win/loss streak
- Board: minions with entity ID, card ID, name, ATK/HP (base + buff), enchantments
- Buffs: 13 categories with per-category ATK/HP totals
- Abilities: spellcraft stacks, economy counters
- Modifications: board-wide stat changes (turn, target, delta, source, category)
- Anomaly: card ID, name, description
- Duos: partner battletag, partner hero, partner tavern tier, partner triples, partner board (last combat snapshot), partner buffs, partner ability counters
- Turn snapshots: full state + buff/ability deltas per turn
- Metadata: game ID, start/end timestamps, placement, is_duos
