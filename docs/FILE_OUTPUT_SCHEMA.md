# File Output Schema

JSON files are written atomically to `~/.battlestream/profiles/<profile>/stats/` (configurable via the profile's `output.path`; the `~/.battlestream` base directory can be overridden with the `BS_CONFIG_DIR` environment variable).

Writes use a `.tmp` then `rename` pattern to prevent partial reads.

## Directory Layout

```
stats/
  current/
    game_state.json
    player_stats.json
    board_state.json
    modifications.json
    buff_sources.json
    partner_stats.json   (Duos games only)
  aggregate/
    summary.json
  history/
    2026-03-04_game-1.json
```

---

## `current/game_state.json`

Updated every `output.write_interval_ms` milliseconds.

```json
{
  "game_id": "game-1",
  "phase": "RECRUIT",
  "turn": 7,
  "tavern_tier": 4,
  "updated_at": "2026-03-04T20:00:00Z"
}
```

### Phases

| Value | Description |
|---|---|
| `IDLE` | No game in progress |
| `LOBBY` | In the lobby / loading |
| `RECRUIT` | Recruit phase (odd turns) |
| `COMBAT` | Combat phase (even turns) |
| `GAME_OVER` | Game ended |

---

## `current/player_stats.json`

```json
{
  "name": "Fixates",
  "hero_card_id": "TB_BaconShop_HERO_08",
  "health": 28,
  "armor": 5,
  "spell_power": 0,
  "triple_count": 2,
  "win_streak": 1,
  "updated_at": "2026-03-04T20:00:00Z"
}
```

`placement` is omitted during an active game. When the game ends it is written as `1–8`.

---

## `current/board_state.json`

```json
{
  "board": [
    {
      "entity_id": 42,
      "card_id": "EX1_506",
      "name": "Murloc Tidehunter",
      "attack": 3,
      "health": 4,
      "minion_type": "MURLOC",
      "buff_attack": 2,
      "buff_health": 1
    }
  ],
  "updated_at": "2026-03-04T20:00:00Z"
}
```

`buff_attack` and `buff_health` are cumulative buffs (Blood Gems, spells, etc.) applied this game.

---

## `current/modifications.json`

```json
{
  "modifications": [
    {
      "turn": 3,
      "target": "ALL",
      "stat": "ATTACK",
      "delta": 1,
      "source": "Blood Gem"
    },
    {
      "turn": 5,
      "target": "BEAST",
      "stat": "HEALTH",
      "delta": 2,
      "source": "Tavish, Master Marksman"
    }
  ],
  "updated_at": "2026-03-04T20:00:00Z"
}
```

---

## `current/buff_sources.json`

Per-category cumulative buff totals for the local player (Blood Gems, Tavern Spells, Nomi, etc.).

```json
{
  "buff_sources": [
    {"category": "BLOODGEM", "attack": 12, "health": 8},
    {"category": "TAVERN_SPELL", "attack": 4, "health": 4}
  ],
  "updated_at": "2026-03-04T20:00:00Z"
}
```

`category` values and their display names are defined in `internal/gamestate/categories.go`. `attack` and `health` are the current effective buff totals for that category.

---

## `current/partner_stats.json`

Written only during Duos games, when a partner has been identified. Same shape as `player_stats.json` (without `placement`).

```json
{
  "name": "Partner#1234",
  "hero_card_id": "TB_BaconShop_HERO_42",
  "health": 28,
  "armor": 5,
  "spell_power": 0,
  "triple_count": 1,
  "win_streak": 0,
  "updated_at": "2026-03-04T20:00:00Z"
}
```

Note: health/armor are a shared team pool in Duos.

---

## `aggregate/summary.json`

```json
{
  "games_played": 11,
  "wins": 8,
  "losses": 3,
  "avg_placement": 2.9,
  "updated_at": "2026-03-04T20:00:00Z"
}
```

A "win" is defined as placement 1–4; a "loss" is placement 5–8.

---

## `history/{date}_{gameID}.json`

Full `BGGameState` snapshot written at game end. Contains all fields from the above schemas plus `end_time_unix`.

---

## OBS Browser Source Integration

Point an OBS browser source at a local HTTP server that serves the `stats/` directory, or use the REST API directly.

Example using Python's built-in HTTP server:

```sh
cd ~/.battlestream/profiles/<profile>/stats
python3 -m http.server 8090
```

Then in OBS, use a browser source with HTML that polls `http://localhost:8090/current/player_stats.json`.

## StreamDeck Plugin Integration

StreamDeck plugins can poll the REST API:
- `GET http://localhost:8080/v1/game/current` — full game state
- `GET http://localhost:8080/v1/stats/aggregate` — session summary

Or connect to `ws://localhost:8080/ws/events` for push updates.
