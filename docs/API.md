# API Reference

## gRPC

Server: `localhost:50051` (configurable via `api.grpc_addr`)

Proto package: `battlestream.v1`

Server reflection is enabled, so `grpcurl` works out of the box:

```sh
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 battlestream.v1.BattlestreamService/GetCurrentGame
grpcurl -plaintext localhost:50051 battlestream.v1.BattlestreamService/GetAggregate
```

### Service: BattlestreamService

| RPC | Request | Response | Description |
|---|---|---|---|
| `GetCurrentGame` | `GetCurrentGameRequest` | `GameState` | Current game state snapshot |
| `GetGame` | `GetGameRequest{game_id}` | `GameState` | Historical game by ID |
| `StreamGameEvents` | `StreamRequest` | `stream GameEvent` | Real-time event stream |
| `GetAggregate` | `GetAggregateRequest` | `AggregateStats` | Aggregate stats across all games (includes `best_placement`, `worst_placement`) |
| `ListGames` | `ListGamesRequest{limit,offset}` | `ListGamesResponse` | Paginated game history |
| `GetPlayerProfile` | `GetPlayerRequest{name}` | `PlayerProfile` | **Not implemented** — returns `UNIMPLEMENTED`. The store does not record player names per game, so per-player filtering is impossible; use `GetAggregate`. The REST mapping `GET /v1/player/{name}` likewise returns `501 Not Implemented`. |

---

## REST

Server: `http://localhost:8080` (configurable via `api.rest_addr`)

All endpoints return `application/json`.

### Authentication

If `api.api_key` is set, include:
```
Authorization: Bearer <key>
```

### Browser access (Origin / Host policy)

The server is intended for localhost use:

- Requests **without** an `Origin` header (curl, native clients) are always accepted.
- WebSocket upgrades and SSE connections **with** an `Origin` header are only accepted from loopback origins (`localhost`, `127.0.0.1`, `[::1]` — any port). Other origins get `403`.
- All requests must carry a `Host` header that is a localhost form or the configured bind host (DNS-rebinding protection). A wildcard bind (`0.0.0.0` / `::`) accepts any host.

### Endpoints

#### `GET /v1/health`
Health check. Always returns `{"status":"ok"}`.

#### `GET /v1/game/current`
Current game state.

#### `GET /v1/game/{game_id}`
Historical game state by ID (same shape as `/v1/game/current`). Returns `404` if the game is not in the store.

#### `GET /v1/game/{game_id}/turns`
Per-turn snapshots for a stored game. Returns an array of turn snapshot objects, each containing `turn`, a full `state` (`BGGameState`) snapshot, and optional `buff_deltas`, `ability_deltas`, and `modifications` for that turn. Returns `[]` if no snapshots exist.

```json
{
  "game_id": "game-1",
  "phase": "RECRUIT",
  "turn": 7,
  "tavern_tier": 4,
  "player": {
    "name": "Fixates",
    "hero_card_id": "TB_BaconShop_HERO_08",
    "health": 28,
    "armor": 5,
    "current_gold": 0,
    "spell_power": 0,
    "triple_count": 2,
    "win_streak": 1,
    "loss_streak": 0
  },
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
  "modifications": [],
  "start_time_unix": 1741108800,
  "placement": 0
}
```

#### `GET /v1/stats/aggregate`
Aggregate stats across all completed games (stale games with placement 0 are excluded).

Query parameters:

| Param | Description |
|---|---|
| `mode` | Filter by game mode: `solo`, `duos`, or `all` (default: all) |

```json
{
  "wins": 8,
  "losses": 3,
  "placements": [1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3],
  "games_played": 11,
  "avg_placement": 2.9
}
```

> For `best_placement` and `worst_placement`, use the gRPC `GetAggregate` RPC.

#### `GET /v1/stats/games`
List of recent games (newest first). Stale games (placement 0) are excluded.

Query parameters:

| Param | Default | Description |
|---|---|---|
| `limit` | `50` | Maximum number of games to return (`0` = no limit) |
| `offset` | `0` | Number of games to skip (offset pagination) |
| `mode` | all | Filter by game mode: `solo`, `duos`, or `all` |

```json
{
  "games": [
    {"game_id": "game-11", "start_time_unix": 1741108800, "end_time_unix": 1741109400, "placement": 1},
    {"game_id": "game-10", "start_time_unix": 1741022400, "end_time_unix": 1741023000, "placement": 3, "is_duos": true}
  ],
  "total": 11
}
```

`total` is the number of games matching the filter *before* pagination, so clients can page off it.

#### `GET /v1/stats/games/{game_id}/modifications`
Stat modifications (board-wide buffs) recorded for a stored game. Returns `404` if the game is not in the store.

```json
{
  "game_id": "game-11",
  "modifications": [
    {"turn": 3, "target": "Board (4x)", "stat": "ATTACK", "delta": 1}
  ]
}
```

#### `GET /v1/player/{name}`
**Not implemented** — returns `501 Not Implemented`. The store does not record player names per game, so per-player filtering is impossible. Use `/v1/stats/aggregate` instead.

#### `GET /v1/cardnames`
Full CardID → friendly name map for all known Battlegrounds cards. Useful for clients that receive raw card IDs from the event stream.

```json
{"BG34_403": "Eternal Tycoon", "EX1_506": "Murloc Tidehunter"}
```

#### `GET /dashboard/`
Built-in web dashboard (static HTML/JS, embedded in the binary). `GET /dashboard` redirects here. Not part of the versioned JSON API.

---

## WebSocket

Endpoint: `ws://localhost:8080/ws/events`

Connect to receive a real-time stream of `GameEvent` JSON objects. Events are pushed as they are parsed from `Power.log`.

```json
{
  "type": "TAG_CHANGE",
  "timestamp": "2026-03-04T20:01:40Z",
  "entity_id": 42,
  "tags": {"HEALTH": "28"},
  "entity_name": "Fixates"
}
```

### Event Types

| Type | Description |
|---|---|
| `GAME_START` | New game started |
| `GAME_END` | Game ended (tags include `PLAYER_LEADERBOARD_PLACE`) |
| `TURN_START` | New turn (tags include `TURN`) |
| `TAG_CHANGE` | Entity tag changed (zone moves arrive as `TAG_CHANGE` with `tag=ZONE`) |
| `ENTITY_UPDATE` | Entity created or updated |
| `PLAYER_DEF` | Player entity definition from the CREATE_GAME block |
| `PLAYER_NAME` | PlayerID → player name mapping |
| `GAME_ENTITY_TAGS` | Tags from the GameEntity block in CREATE_GAME |

### Event Fields

| Field | Type | Description |
|---|---|---|
| `type` | string | One of the event types above |
| `timestamp` | string (RFC3339) | When the log line was written |
| `entity_id` | int | Entity ID (omitted if 0) |
| `player_id` | int | CONTROLLER / `player=` field (omitted if 0) |
| `tags` | object | Key-value tag map (omitted if empty) |
| `entity_name` | string | Entity display name (omitted if empty) |
| `card_id` | string | Hearthstone card ID (omitted if empty) |
| `block_source` | int | Entity ID from the enclosing `BLOCK_START` (omitted if 0) |
| `block_card_id` | string | Card ID from the enclosing `BLOCK_START` (omitted if empty) |

---

## SSE

Endpoint: `http://localhost:8080/v1/events`

Standard Server-Sent Events stream. Same event format as WebSocket.

```sh
curl -N http://localhost:8080/v1/events
```

Each event:
```
data: {"type":"TURN_START","timestamp":"2026-03-04T20:01:40Z","tags":{"TURN":"3"}}

```
