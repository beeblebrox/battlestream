# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```sh
# Build
go build ./cmd/battlestream

# Test (matches CI)
go test -race -count=1 ./...

# Test a single package
go test -race -count=1 ./internal/gamestate/

# Test a single test function
go test -race -count=1 -run TestProcessorIntegration ./internal/gamestate/

# Lint (CI uses golangci-lint)
go vet ./...

# Regenerate protobuf code (run after editing proto/*.proto)
scripts/gen-proto.sh
```

**Before committing and pushing**, always run `go vet ./...` to catch issues like `copylocks`, `printf` format mismatches, and unreachable code. CI runs both `go vet` and `golangci-lint` — if `golangci-lint` is installed locally, run `golangci-lint run` as well. This prevents avoidable CI failures after push.

## Architecture

Hearthstone Battlegrounds stat tracker. The daemon tails `Power.log`, parses game events, maintains live state, and exposes it via multiple APIs.

**Data pipeline:** `watcher` (tail Power.log) -> `parser` (regex -> GameEvent) -> `gamestate.Processor` (state machine) -> outputs (fileout, gRPC, REST/WS/SSE)

Key packages under `internal/`:
- **parser** — Regex-based line parser. Only processes `GameState` debug lines; filters out `PowerTaskList`. Outputs typed `GameEvent` structs.
- **gamestate** — Core state machine. `Machine` holds `BGGameState` (thread-safe via RWMutex). `Processor` applies events. Entity registry (`entityProps`) tracks ATK/HEALTH/ZONE/CARDTYPE per entity ID. `categories.go` maps CardIDs to buff source categories.
- **store** — BadgerDB v4 persistence with dedup (`HasGame`).
- **tui** — Live Bubbletea dashboard connecting to daemon via gRPC. `--dump` flag renders to stdout for testing.
- **debugtui** — Offline step-through Power.log replay viewer. Builds a `Replay` (slice of `Step` snapshots).
- **api/grpc** — gRPC server on :50051 with reflection. Generated code in `internal/api/grpc/gen/`.
- **api/rest** — REST on :8080 + WebSocket (`/ws/events`) + SSE (`/v1/events`).

Entry point: `cmd/battlestream/main.go` (cobra subcommands: daemon, tui, replay, discover, config, reparse, db-reset, version).

## Parser/Gamestate Conventions

- Game end is detected by `TAG_CHANGE Entity=GameEntity tag=STATE value=COMPLETE` (not GAME_RESULT).
- Local player identified via `GameAccountId=[hi=<nonzero>]` in CREATE_GAME.
- GameEntity TURN tag is doubled (odd=recruit, even=combat); player TURN is the real BG turn number.
- Board state tracks combat copies: created SETASIDE with base stats, buffed via TAG_CHANGE, then ZONE=PLAY.
- ZONE=SETASIDE transitions remove minions from board (like GRAVEYARD/REMOVEDFROMGAME).
- Minion removal is blocked during GAME_OVER phase to preserve final board.
- Triples tracked via `PLAYER_TRIPLES` tag (absolute value).
- Modifications: only board-wide buffs (2+ minions with same turn/stat/delta) are recorded.

## Board Snapshot Invariants

The board snapshot (`Machine.boardSnapshot`) preserves the recruit-phase board for restoration at game end (combat replaces minions with simulation copies that have base stats).

**Critical rules for any code that mutates the board during recruit phase:**
- After `UpsertMinion()` → call `UpdateBoardSnapshot()` if phase is RECRUIT.
- After `RemoveMinion()` → call `UpdateBoardSnapshot()` if phase is RECRUIT.
- After `UpdateMinionStat()` returns true → call `UpdateBoardSnapshot()` if phase is RECRUIT.
- The snapshot MUST be a deep copy (including `Enchantments` slices) to avoid shared backing arrays.
- During COMBAT phase, do NOT sync the snapshot — it preserves the recruit board intentionally.
- During GAME_OVER phase, do NOT add/remove minions — the restored snapshot is the final board.

## Buff Source Tracking

Covers all 13 HDT BgCounters. Four tracking mechanisms:
1. **Player tags** — `BACON_BLOODGEM*`, `BACON_ELEMENTAL_*`, `TAVERN_SPELL_*` -> BuffSource (ATK/HP)
2. **Player tags (economy)** — `BACON_FREE_REFRESH_COUNT`, `BACON_PLAYER_EXTRA_GOLD_NEXT_TURN` -> AbilityCounter
3. **Dnt enchantment SD** — `BG_ShopBuff_Elemental`, `BG31_808pe`, `BG34_854pe`, etc. -> BuffSource
4. **Zone-tracked enchantments** — `BG28_884e` (Overconfidence) PLAY transitions -> GoldNextTurn counter
5. **Numeric tag 3809** — Spellcraft stacks -> AbilityCounter

Mappings live in `internal/gamestate/categories.go`. Reference implementations in `reference/Hearthstone-Deck-Tracker/` and `reference/HearthDb/`.

**Duos Dnt enchantment fix:** In duos, Dnt enchantments (e.g. `BG_ShopBuff`) may have `CONTROLLER=<botID>` but are `ATTACHED` to the local hero. The `isLocalDntTarget()` helper accepts these regardless of CONTROLLER.

## Duos Support

Duos detection signal hierarchy (HS patch 2026-03 changed `BACON_DUOS_PUNISH_LEAVERS` to appear in ALL BG games):
1. `BACON_DUO_TEAMMATE_PLAYER_ID` in CREATE_GAME Player block — **authoritative**, immediately sets duos
2. `BACON_DUOS_PUNISH_LEAVERS=1` + `BACON_DUO_PASSABLE=1` combined — backup; neither alone is sufficient
3. `BACON_DUOS_PUNISH_LEAVERS=0` TAG_CHANGE — clears duos if set only via backup path (not TEAMMATE_PLAYER_ID)

Partner hero identified by `PLAYER_ID` tag in FULL_ENTITY (not CONTROLLER, which is a shared bot ID). Health/armor are shared (team pool).

**Deferred partner resolution:** When duos is detected without `BACON_DUO_TEAMMATE_PLAYER_ID`, the partner is resolved later via `BACON_CURRENT_COMBAT_PLAYER_ID`. The `resolvePartner()` method scans hero entities for matching PLAYER_ID.

**Partner board (last seen):** Captured from combat copy entities during partner's combat. When `BACON_CURRENT_COMBAT_PLAYER_ID` matches the partner, the processor tracks the partner hero copy's CONTROLLER and collects MINION entities with the same controller. Stored in `PartnerBoard` (Minions, Turn, Stale). Shown in both TUIs.

**Available from Power.log:** Partner hero name/CardID, tavern tier, triples, armor, damage, board (last seen from combat copies). Shown in TUI hero panel and partner board panel.

**Not available (requires memory reading):** Partner buff sources, ability counters, gold. Live partner board (only combat snapshots available).

**Stale game timeout:** Games with no events for 3 minutes are forced to GAME_OVER (placement=0). `CheckStaleness()` is called from the daemon's periodic ticker.

**Reconnect handling:** Mid-game reconnects emit a new CREATE_GAME. Player entity tags (TURN, RESOURCES) and hero FULL_ENTITY tags (DAMAGE, ARMOR, PLAYER_TECH_LEVEL, PLAYER_TRIPLES) are captured to restore state.

## Proto / Code Generation

Proto definitions: `proto/battlestream/v1/*.proto`. Generated Go code: `internal/api/grpc/gen/`. CI verifies generated files are up to date. Run `scripts/gen-proto.sh` after any proto changes.

## Module & Go Version

Module path: `battlestream.fixates.io`. Targets Go 1.24 (per go.mod).

## CI

GitHub Actions (`.github/workflows/ci.yml`): Build & Test, Lint (golangci-lint), Proto check, Docker build.
