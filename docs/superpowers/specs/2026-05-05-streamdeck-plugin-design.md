# Stream Deck Plugin — Design Spec

**Date:** 2026-05-05  
**Status:** Approved

---

## Overview

A Stream Deck plugin that displays live Hearthstone Battlegrounds stats from the battlestream daemon. Each tracked stat is a discrete action the user can drag onto any key. An Auto-Layout action fills the entire profile with a curated set of buttons in one click.

---

## Goals

- One action per stat — zero configuration, drop and done
- Auto-Layout fills the whole profile for the current device type
- Real-time updates via SSE with automatic reconnect
- Works across all Stream Deck form factors
- Lives in the battlestream repo, built and packaged with it

---

## SDK & Platform

| Property | Value |
|---|---|
| SDK | `@elgato/streamdeck` v2.1.0 (official Elgato SDK) |
| SDKVersion | 3 |
| Language | TypeScript |
| Bundler | Rollup |
| Node.js | 24+ |
| Stream Deck app | 7.1+ |
| Plugin UUID | `com.battlestream.streamdeck` |

---

## Target Devices

All Stream Deck form factors. Auto-layout profiles are bundled per device type:

| Device | Grid | Bundled profile |
|---|---|---|
| Stream Deck (Standard) | 5×3 (15 keys) | `battlestream-standard.sdProfile` |
| Stream Deck XL | 8×4 (32 keys) | `battlestream-xl.sdProfile` |
| Stream Deck Mini | 3×2 (6 keys) | `battlestream-mini.sdProfile` |
| Stream Deck + | 4×3 (12 keys) | `battlestream-plus.sdProfile` |

SD+ dials (encoder controller) are out of scope for v1.

---

## Data Source

### Connection
- **Initial state:** `GET http://{host}:{port}/v1/game/current` on plugin start
- **Live updates:** SSE stream at `GET http://{host}:{port}/v1/events`
- **Auth:** `Authorization: Bearer {apiKey}` header — omitted entirely when API key is blank
- **Reconnect:** exponential backoff (500ms → 1s → 2s → … → 30s cap), indefinite retries

### State Fan-Out
A singleton `BattlestreamClient` owns the SSE connection. On each SSE event it updates the in-memory `StateStore`, which notifies all currently-visible action instances via a subscription pattern. Actions subscribe in `onWillAppear` and unsubscribe in `onWillDisappear`.

---

## Actions

### Stat Actions (15)

Each action extends `BaseStat`, which handles subscribe/unsubscribe and calls `render()` on state changes.

| Action name | Stat field | Value | Subtitle |
|---|---|---|---|
| Health | `player_stats.health` | `32` | `/ {max_health}` |
| Armor | `player_stats.armor` | `5` | — |
| Tavern Tier | `player_stats.tavern_tier` | `4` | — |
| Gold | `player_stats.current_gold` | `7` | `/ {max_gold}` |
| Triple Count | `player_stats.triple_count` | `2` | — |
| Win Streak | `player_stats.win_streak` | `3` | — |
| Loss Streak | `player_stats.loss_streak` | `0` | — |
| Placement | `player_stats.placement` | `—` (live) or `3rd` (final) | — |
| Spell Power | `player_stats.spell_power` | `0` | — |
| Turn | `game_state.turn` | `8` | — |
| Phase | `game_state.phase` | `RECRUIT` or `COMBAT` | — |
| Minion Count | `len(game_state.board)` | `6` | `/ 7` (max hardcoded) |
| Total Buff ATK | `sum(buff_sources[*].attack)` | `+14` | — |
| Total Buff HP | `sum(buff_sources[*].health)` | `+8` | — |
| Anomaly | `game_state.anomaly_name` | truncated to ~10 chars | — |

**XL bonus actions** (shown in XL profile, also manually droppable):

| Action name | Source |
|---|---|
| Bloodgem Buff | `buff_sources` where category = `BLOODGEM` |
| Elemental Buff | `buff_sources` where category = `ELEMENTAL` |
| Spellcraft | `ability_counters` where category = `SPELLCRAFT` |
| Tavern Spell Buff | `buff_sources` where category = `TAVERN_SPELL` |

### Auto-Layout Action

When pressed, detects the current device type via the event context and calls `streamDeck.profiles.switchToProfile(deviceId, profileName)` with the matching bundled profile. The bundled profiles ship inside the `.sdPlugin` bundle and are pre-configured with all stat buttons placed.

**No-game state:** When no game is active (`/v1/game/current` returns empty or daemon is unreachable), buttons show a muted "—" value and a dim disconnected indicator (desaturated background).

---

## Button Visual Design

Each button is rendered to a 144×144 canvas (2× HiDPI) and sent via `setImage()` as a base64 PNG.

### Layout (per key)
```
┌─────────────────┐
│   STAT NAME     │  ← 11px, uppercase, rgba(255,255,255,0.70), top 22%
│      32         │  ← 48px, bold white, center
│    / 40         │  ← 10px, rgba(255,255,255,0.55), bottom 20%
└─────────────────┘
```

### Color palette (per stat)

| Stat | Gradient |
|---|---|
| Health | `#7b0000` → `#c0392b` |
| Armor | `#3d0000` → `#922b21` |
| Tavern Tier | `#1a3a00` → `#27ae60` |
| Gold | `#5c4a00` → `#d4a017` |
| Triples | `#2d0060` → `#8e44ad` |
| Win Streak | `#003366` → `#2980b9` |
| Loss Streak | `#4a2000` → `#e67e22` |
| Placement | `#00474a` → `#16a085` |
| Spell Power | `#4a004a` → `#a93226` |
| Turn | `#1a1a3a` → `#5d6d7e` |
| Phase | `#1a0030` → `#6c3483` |
| Minion Count | `#003030` → `#1abc9c` |
| Buff ATK | `#3a1000` → `#cb4335` |
| Buff HP | `#3a003a` → `#c0392b` |
| Anomaly | `#1a1a1a` → `#566573` |
| Auto-Layout | `#4a3800` → `#c8960c` |

---

## Global Settings

Configured once in the Stream Deck software (applies to all Battlestream buttons):

| Setting | Default | Notes |
|---|---|---|
| Host | `127.0.0.1` | Daemon host |
| Port | `8080` | Daemon REST port |
| API Key | *(blank)* | Omit header when blank; use with `--no-auth` for local use |

---

## Backend Change — `--no-auth` Flag

New flag on the `daemon` subcommand:

```
battlestream daemon --no-auth
```

- `noAuth bool` added to daemon config / Cobra flags
- Also configurable via env var `BS_NO_AUTH=true`
- When `true`, `withAuth` middleware passes all requests through without checking the Bearer token
- Default: `false` (existing behavior unchanged)
- Documented in `battlestream daemon --help`

---

## Project Structure

```
streamdeck-plugin/
├── src/
│   ├── plugin.ts              # Entry — register actions, create client + store
│   ├── client.ts              # SSE client, auto-reconnect
│   ├── state.ts               # GameState store, subscription fan-out
│   ├── render.ts              # Canvas → base64 PNG renderer
│   └── actions/
│       ├── base.ts            # BaseStat base class
│       ├── health.ts
│       ├── armor.ts
│       ├── tavern-tier.ts
│       ├── gold.ts
│       ├── triples.ts
│       ├── win-streak.ts
│       ├── loss-streak.ts
│       ├── placement.ts
│       ├── spell-power.ts
│       ├── turn.ts
│       ├── phase.ts
│       ├── minion-count.ts
│       ├── buff-atk.ts
│       ├── buff-hp.ts
│       ├── anomaly.ts
│       └── auto-layout.ts
├── ui/
│   └── global-settings.html   # Property inspector for global plugin settings
├── profiles/
│   ├── battlestream-standard.sdProfile
│   ├── battlestream-xl.sdProfile
│   ├── battlestream-mini.sdProfile
│   └── battlestream-plus.sdProfile
├── imgs/
│   ├── plugin-icon.png        # Shown in SD action list category header
│   └── actions/               # Per-action icons (72×72 and 144×144)
├── manifest.json
├── package.json
├── tsconfig.json
└── rollup.config.mjs
```

---

## Build & Packaging

### Local build

```sh
# Build plugin only
scripts/build-plugin.sh        # cd streamdeck-plugin && npm ci && npm run build

# Build everything
make build-all                 # go build ./cmd/battlestream + build-plugin.sh
```

### CI — new job: `plugin`

```yaml
- name: Build Stream Deck plugin
  working-directory: streamdeck-plugin
  run: |
    npm ci
    npm run build
```

Runs independently of the Go jobs. No artifact upload in CI (binary-only releases attach the zip).

### GitHub Releases

The `.sdPlugin` output directory is zipped to `battlestream-streamdeck-vX.Y.Z.zip` and attached to the release alongside the Go binary. Users install by double-clicking the zip (Stream Deck software handles installation).

---

## Error States

| Condition | Button appearance |
|---|---|
| Daemon unreachable | Desaturated gradient, `—` value, `OFFLINE` subtitle |
| No active game | Normal color, `—` value, `NO GAME` subtitle |
| SSE reconnecting | Normal color, last known value, subtitle cycles `·` `··` `···` via periodic `setTitle()` |
| Game over | Normal color, final value, `GAME OVER` subtitle |

---

## Out of Scope (v1)

- SD+ dial / encoder support
- Per-button color customization
- Individual buff source buttons on non-XL devices
- Board minion display (not feasible at 72×72)
- Partner stats
