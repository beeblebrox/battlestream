# Architecture

## Overview

```
Hearthstone
    │ Power.log / Zone.log
    ▼
┌─────────────────────────────────────────────────────────────────┐
│                        battlestream daemon                      │
│                                                                 │
│  watcher ──► parser ──► gamestate.Machine ──► fileout.Writer   │
│                │                  │                             │
│                ▼                  ▼                             │
│          event channel      store (BadgerDB)                    │
│                │                  │                             │
│                ▼                  ▼                             │
│         grpc.Server ◄────────────────────────────────          │
│                │                                                │
│                ▼                                                │
│          rest.Server                                            │
│         ├── GET /v1/game/current                                │
│         ├── GET /v1/stats/aggregate                             │
│         ├── GET /v1/stats/games                                 │
│         ├── WS  /ws/events                                      │
│         └── SSE /v1/events                                      │
└─────────────────────────────────────────────────────────────────┘
           │             │                │
     StreamDeck      OBS Browser      battlestream tui
      (REST/WS)        (SSE/WS)          (gRPC)
```

## Components

### `internal/watcher`
Tails `Power.log` using `nxadm/tail` with inotify. Emits raw log lines on a `chan watcher.Line`.

### `internal/parser`
Regex-based parser that converts raw lines to typed `parser.GameEvent` structs. Handles: game start/end, turn changes, entity updates, tag changes, and zone changes.

### `internal/gamestate`
State machine (`Machine`) that applies `GameEvent`s to an in-memory `BGGameState`. Thread-safe via `sync.RWMutex`. The `Processor` type wires parser events to the machine.

### `internal/store`
BadgerDB v4 embedded database. Stores game metadata and history. Provides aggregate stat queries.

### `internal/fileout`
Atomically writes JSON stat files (`write → .tmp → rename`) to the configured output directory. Used by OBS overlays and StreamDeck plugins that prefer file-based polling.

### `internal/api/grpc`
gRPC server on `localhost:50051`. Implements `BattlestreamService`. Maintains a fan-out event broadcaster for streaming RPCs.

### `internal/api/rest`
HTTP server on `localhost:8080`. Provides:
- REST endpoints via `net/http`
- WebSocket hub (gorilla/websocket) at `/ws/events`
- SSE endpoint at `/v1/events`

### `internal/tui`
Live dashboard. Connects to the daemon via gRPC and renders board state, buff
sources, and session stats in a scrollable two-column layout. Panels use
`bubbles/viewport` with visual scrollbars; mouse-wheel and scrollbar
click/drag scrolling are supported. In Duos games, the hero panel includes a
partner section showing hero name, tavern tier, and triple count. Launched
via `battlestream tui`.

### `internal/debugtui`
Step-through Power.log replay viewer. Parses a log file offline into a `Replay`
(slice of `Step` snapshots) and lets the user step through events, filter by
type, and jump to any BG turn. Panels: header, player stats, board, buff
sources, changes diff, event summary, raw log — all viewports with scrollbars
and full mouse support.

Launch interactively: `battlestream replay <log-file>` (or `battlestream debug`)
Dump to stdout: `battlestream replay --dump --turn N <log-file>`

Both TUIs share the same panel conventions (colours, borders, viewport
scrollbars, mouse routing). See [docs/DEBUGTUI.md](DEBUGTUI.md) for the full
design reference covering panel sizing, scrollbar mechanics, mouse routing,
Bubbles components, and the rules for adding new panels to either TUI.

### `internal/discovery`
Cross-platform Hearthstone install detection. Searches platform-specific paths and falls back to interactive user input.

### `internal/logconfig`
Reads and patches `log.config` to enable the verbose logging sections required for parsing.

### `internal/config`
Viper-based config loading from YAML file + environment variables (`BS_*` prefix).

## Data Flow

1. Hearthstone writes to `Power.log`
2. `watcher` detects new lines via inotify/fsnotify
3. Raw lines flow to `parser`
4. `parser` emits `GameEvent`s
5. `gamestate.Processor` applies events to `Machine`
6. On a timer (`write_interval_ms`), `fileout.Writer` snapshots current state to JSON
7. `grpc.Server` fans events out to streaming gRPC clients and the REST layer
8. `rest.Server` broadcasts events over WebSocket and SSE
9. On game end, `store` persists the game record and `fileout` writes history
