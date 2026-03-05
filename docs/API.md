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
| `GetPlayerProfile` | `GetPlayerRequest{name}` | `PlayerProfile` | Per-player profile |

---

## REST

Server: `http://localhost:8080` (configurable via `api.rest_addr`)

All endpoints return `application/json`.

### Authentication

If `api.api_key` is set, include:
```
Authorization: Bearer <key>
```

### Endpoints

#### `GET /v1/health`
Health check. Always returns `{"status":"ok"}`.

#### `GET /v1/game/current`
Current game state.

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
Aggregate stats across all games.

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
List of recent games (newest first, up to 50 results).

```json
[
  {"game_id": "game-11", "start_time_unix": 1741108800, "end_time_unix": 1741109400, "placement": 1},
  {"game_id": "game-10", "start_time_unix": 1741022400, "end_time_unix": 1741023000, "placement": 3}
]
```

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
| `TAG_CHANGE` | Entity tag changed |
| `ENTITY_UPDATE` | Entity created or updated |
| `ZONE_CHANGE` | Entity moved between zones |
| `PLAYER_UPDATE` | Player stat changed |

### Event Fields

| Field | Type | Description |
|---|---|---|
| `type` | string | One of the event types above |
| `timestamp` | string (RFC3339) | When the log line was written |
| `entity_id` | int | Entity ID (omitted if 0) |
| `tags` | object | Key-value tag map (omitted if empty) |
| `entity_name` | string | Entity display name (omitted if empty) |
| `card_id` | string | Hearthstone card ID (omitted if empty) |

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
