# bscapture: Screenshot Capture Binary Design

**Date:** 2026-03-27
**Status:** Approved

## Overview

Standalone binary (`bscapture`) that captures timed screenshots of Hearthstone Battlegrounds during active games, tagged with full game state metadata. Purpose: post-game timeline analysis and debugging battlestream state tracking issues.

## Binary & Config

**Binary:** `cmd/bscapture/main.go` → `bscapture`

**Subcommands (cobra):**
- `bscapture run` — start capture session (main loop)
- `bscapture detect` — interactive monitor detection, writes config
- `bscapture list` — list captured game sessions
- `bscapture config` — show/set config values

**Config:** viper, YAML at `~/.config/bscapture/config.yaml`, env prefix `BSC_`

```yaml
power_log_path: "/path/to/Power.log"    # auto-discovered or set via detect
monitor: "DP-1"                          # from detect subcommand
capture_interval: "1s"                   # duration
output_resolution: "1920x1080"           # scale target
jpeg_quality: 92
data_dir: "~/.local/share/bscapture"
stale_timeout: "5m"                      # no events = stop recording
```

## Architecture: Embedded Pipeline

The binary embeds battlestream's parser and gamestate packages directly, communicating through small interfaces:

```go
// EventSource produces parsed game events from Power.log
type EventSource interface {
    Events() <-chan parser.GameEvent
    Close() error
}

// StateTracker maintains game state from events
type StateTracker interface {
    Apply(event parser.GameEvent)
    Snapshot() CaptureState  // point-in-time copy under lock
    InGame() bool
}

// Screenshotter captures the display
type Screenshotter interface {
    Capture(ctx context.Context) (image.Image, error)
}

// FrameStore persists frames and metadata
type FrameStore interface {
    InitGame(gameID string) error
    SaveFrame(frame Frame) error
    Close() error
}
```

**Concrete implementations:**
- `EventSource` — wraps `watcher` + `parser` (tail Power.log → parse → emit events)
- `StateTracker` — wraps `gamestate.Machine` + `gamestate.Processor`. `Snapshot()` grabs read lock, deep-copies state into flat `CaptureState`
- `Screenshotter` — shells out to `grim -o <monitor> -` (stdout), pipes into Go `image.Decode`
- `FrameStore` — SQLite + filesystem

**CaptureState struct** — point-in-time metadata snapshot:

```go
type CaptureState struct {
    GameID        string
    Timestamp     time.Time
    Turn          int
    Phase         string  // RECRUIT, COMBAT, GAME_OVER
    TavernTier    int
    Health        int
    Armor         int
    Gold          int
    Placement     int
    IsDuos        bool
    PartnerHealth int
    PartnerTier   int
    Board         []MinionSnapshot  // card_id, name, attack, health, tribes, buff_atk, buff_hp
    BuffSources   []BuffSourceSnapshot
}
```

## Capture Loop & Lifecycle

```
Start → Watch Power.log → Wait for game start → Capture loop → Game end/stale → Wait for next game
```

**Per-tick sequence (critical: metadata snapshot at capture time):**
1. Lock state machine, deep-copy `CaptureState`, unlock
2. Shell out to `grim -o <monitor> -` → raw image on stdout
3. Decode image, scale to configured resolution using `golang.org/x/image/draw` (Lanczos)
4. Encode JPEG at configured quality
5. Write file to `<data_dir>/<game_id>/frames/<sequence>.jpg`
6. Insert metadata row into SQLite with the `CaptureState` snapshot
7. Log capture latency; warn if capture took longer than interval

**Game detection:** `StateTracker.InGame()` returns true when phase is not idle (between GAME_START and GAME_OVER/stale timeout).

**Stale timeout:** Goroutine watches time-since-last-event. If exceeds `stale_timeout` (default 5m) during active game, game is ended and capture stops. Resets on new game.

**Graceful shutdown:** SIGINT/SIGTERM closes watcher, flushes pending writes, closes SQLite.

## SQLite Schema

One `.db` file per game at `<data_dir>/<game_id>/capture.db`.

```sql
CREATE TABLE schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE games (
    game_id      TEXT PRIMARY KEY,
    start_time   TEXT NOT NULL,       -- RFC3339
    end_time     TEXT,
    is_duos      BOOLEAN NOT NULL DEFAULT 0,
    placement    INTEGER DEFAULT 0,
    total_frames INTEGER DEFAULT 0
);

CREATE TABLE frames (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id            TEXT NOT NULL REFERENCES games(game_id),
    sequence           INTEGER NOT NULL,  -- 0-indexed frame number
    timestamp          TEXT NOT NULL,      -- RFC3339Nano
    elapsed_ms         INTEGER NOT NULL,   -- ms since game start
    file_path          TEXT NOT NULL,      -- relative path to JPEG
    file_size_bytes    INTEGER NOT NULL,
    capture_latency_ms INTEGER NOT NULL,

    -- game state at capture time
    turn               INTEGER NOT NULL,
    phase              TEXT NOT NULL,      -- RECRUIT, COMBAT, GAME_OVER
    tavern_tier        INTEGER NOT NULL,
    health             INTEGER NOT NULL,
    armor              INTEGER NOT NULL,
    gold               INTEGER NOT NULL,
    placement          INTEGER NOT NULL DEFAULT 0,

    -- duos
    is_duos            BOOLEAN NOT NULL DEFAULT 0,
    partner_health     INTEGER,
    partner_tier       INTEGER,

    -- JSON columns for structured data
    board_json         TEXT NOT NULL,      -- []MinionSnapshot with tribes
    buff_sources_json  TEXT NOT NULL,      -- []BuffSourceSnapshot

    UNIQUE(game_id, sequence)
);

CREATE INDEX idx_frames_game_turn ON frames(game_id, turn);
CREATE INDEX idx_frames_game_phase ON frames(game_id, phase);
```

**Board JSON per minion:**
```json
{"card_id":"BGS_004","name":"Wrath Weaver","attack":25,"health":65,"tribes":["DEMON"],"buff_attack":24,"buff_health":61}
```

### Migration Planning

- `schema_version` table tracks current schema version (starts at 1)
- On DB open, compare current version to latest, run sequential migration functions
- Each migration is a named function (`migrateV1ToV2`, etc.) registered in an ordered slice
- Columns should only be added, never removed, in migrations
- JSON columns (`board_json`, `buff_sources_json`) provide flexibility for evolving nested structures without schema changes
- New structured data should prefer JSON columns over new relational columns unless query performance requires indexing

## Package Layout

```
cmd/bscapture/
    main.go          -- cobra root + subcommands (run, detect, list, config)

internal/capture/
    capture.go       -- interfaces (EventSource, StateTracker, Screenshotter, FrameStore)
    state.go         -- CaptureState, MinionSnapshot structs
    loop.go          -- main capture loop orchestration
    grim.go          -- Screenshotter impl (grim + scale)
    store.go         -- FrameStore impl (SQLite + filesystem)
    migrate.go       -- schema versioning & migrations
    pipeline.go      -- EventSource + StateTracker wrappers over parser/gamestate
```

**New dependencies:**
- `modernc.org/sqlite` — pure-Go SQLite driver (no CGo)
- `golang.org/x/image` — Lanczos scaling

**Reused from battlestream:**
- `internal/parser` — regex parser, GameEvent types
- `internal/gamestate` — Machine, Processor, phase constants
- `internal/watcher` — Power.log tailing
- `internal/discovery` — HS install path detection
- `internal/config` — viper config pattern (separate file for bscapture)

## `detect` Subcommand

1. Run `hyprctl monitors -j` → parse JSON for monitor list
2. Display numbered list: name, resolution, description
3. Optionally detect Hearthstone window via `hyprctl clients -j` → suggest its monitor
4. Prompt user to pick one (stdin)
5. Discover Power.log path via `internal/discovery`
6. Write selections to `~/.config/bscapture/config.yaml`

## System Requirements

- Wayland/Hyprland (grim for capture, hyprctl for monitor detection)
- `grim` installed at `/usr/bin/grim`
- openSUSE Tumbleweed (current dev system)

## Future Considerations

- **Event-driven capture (Approach C):** Add event markers to the DB between frames for finer-grain analysis during tavern phase. The poll loop and event stream are independent concerns — A evolves into C without rewrites.
- **Per-event screenshot triggers:** The embedded pipeline gives direct access to the event stream for future per-event capture during specific phases.
