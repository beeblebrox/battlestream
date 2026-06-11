# 37 — GameNetLogger.log Disconnect/Reconnect Signals

**Priority:** MEDIUM
**Status:** NOT STARTED

## Problem

battlestream only watches Power.log (and Player.log on macOS). GameNetLogger.log
contains valuable disconnect/reconnect metadata that could improve game lifecycle
handling:

- `DisconnectFromGameServer() - Reason: DisconnectAfterFailedPings` — network loss
- `DisconnectFromGameServer() - Reason: EndGameScreen` — normal post-game disconnect
- `DisconnectFromGameServer() - Reason: GameCanceled` — server rejected reconnect
- `reconnecting=True` — client is reconnecting to an in-progress game
- `m_findGameState = SERVER_GAME_CANCELED` — game ended server-side during disconnect

## Evidence from Real Sessions

### Session `Hearthstone_2026_03_16_22_05_57` (Reconnect)
- **GameNetLogger.log**: `reconnecting=True`, same game=502 server
- **LoadingScreen.log**: `IsPastBeginPhase()=True` (HDT uses this as reconnect signal)
- **Power.log**: CREATE_GAME with mid-game state (TURN=7, STATE=RUNNING)
- New session directory was created (client had restarted)

### Session `Hearthstone_2026_03_16_22_58_06` (Post-game AFK)
- **GameNetLogger.log**: `DisconnectAfterFailedPings` 27s after STATE=COMPLETE
- Then `Reason: EndGameScreen` — benign, game already finished
- **Hearthstone.log**: `Error.AddFatal()` — inactivity timeout 30 min later

### Session `Hearthstone_2026_03_17_19_33_31` (Mid-game crash, failed reconnect)
- **GameNetLogger.log**: `DisconnectAfterFailedPings` at 20:46:41 (turn 18)
- First reconnect succeeded briefly (38 seconds) then disconnected again
- Second reconnect: `Reason: GameCanceled` — server gave up
- `m_bypassGameReconnect = True`, `m_findGameState = SERVER_GAME_CANCELED`
- **Power.log**: 106MB, ends abruptly. No STATE=COMPLETE. No second CREATE_GAME
  (reconnect disconnected before server sent state dump)

## Key Findings

1. **Reconnects create new session dirs** if client restarted. Watcher already
   handles this via fsnotify `watchForNewSessions()`.
2. **Soft reconnects** (no restart) append to same Power.log with new CREATE_GAME.
3. **Failed reconnects** produce no new Power.log data — game just stops.
4. **`IsPastBeginPhase()=True`** in LoadingScreen.log is how HDT detects reconnects.
5. **No midnight rotation** — session dirs only created on process start.

## Potential Use

- Detect game cancellation without waiting for staleness timeout
- Flag reconnected games in stats (HDT marks them as potentially invalid)
- Show "Disconnected" / "Reconnecting" status in TUI instead of stale board

## Current Mitigation

Design doc `docs/plans/2026-03-19-duos-tui-design.md` includes a staleness timer
(3 min no events -> mark game over) which handles the user-facing symptoms without
parsing GameNetLogger.log.
