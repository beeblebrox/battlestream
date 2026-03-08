# Fix Plans — Prioritized

Items ordered by criticality. See individual files for detail.

## Priority legend
- **CRITICAL** — data corruption, wrong results, silent misattribution
- **HIGH** — reliability failures, incorrect feature output, memory hazards
- **MEDIUM** — broken declared features, daemon stability, consumer-visible gaps
- **LOW** — fragility, future-proofing, performance, observability

---

## Active TODOs

Active work items tracking multi-step efforts. Each is a self-contained set of
tasks with a log of what is done and what remains.

| File | Status | Summary |
|------|--------|---------|
| [TODO-01-test-suite.md](TODO-01-test-suite.md) | IN PROGRESS | Integration test suite against 2026-03-07 game log |
| [TODO-02-hero-identification.md](TODO-02-hero-identification.md) | DONE | Hero entity identification (placeholder→real, ghost battle safety) |
| [TODO-03-card-friendly-names.md](TODO-03-card-friendly-names.md) | DONE | Card ID → friendly name in all TUIs; `/gen-card-names` skill |
| [TODO-04-buff-source-display.md](TODO-04-buff-source-display.md) | DONE | BG_ShopBuff DNT tracking; display name single source of truth; jumpToTurn end-of-turn fix |
| [TODO-05-spells-played-counter.md](TODO-05-spells-played-counter.md) | DONE | Spells Played counter mislabeled "Naga Spells"; shown even when no relevant minions on board |
| [TODO-06-tui-improvements.md](TODO-06-tui-improvements.md) | DONE | TUI: show available factions, clarify Bonus Gold/Overconfidence, show win/loss, rename Spell+ |
| [TODO-07-tribes-discovery.md](TODO-07-tribes-discovery.md) | DONE | Investigate earlier tribe discovery — HDT uses memory reflection; no early log tag exists |

---

## CRITICAL

| # | File | Issue |
|---|------|-------|
| 01 | [01-parseint-negative.md](01-parseint-negative.md) | ~~`parseInt` ignores minus sign — negative tag values silently become positive~~ **DONE** — uses `strconv.Atoi` |
| 02 | [02-parser-state-reset.md](02-parser-state-reset.md) | ~~Parser block state not reset on `EventGameStart` — stale state corrupts next game~~ **DONE** — block state reset on CREATE_GAME |
| 03 | [03-catspellcraft-rename.md](03-catspellcraft-rename.md) | ~~`CatSpellcraft` tracks Naga spells, not Spellcraft keyword~~ **DONE** — renamed `CatNagaSpells` |
| 04 | [04-tavern-tier-attribution.md](04-tavern-tier-attribution.md) | ~~`handleTagChange` may apply opponent's tier to local player when `controllerID == 0`~~ **DONE** — controllerID!=0 guard added |
| 34 | [34-hero-damage-tracking.md](34-hero-damage-tracking.md) | ~~Hero DAMAGE tag not tracked — health never updates~~ **DONE** — DAMAGE tag handled, effective HP = HEALTH - DAMAGE |

## HIGH

| # | File | Issue |
|---|------|-------|
| 05 | [05-pending-stat-changes-bound.md](05-pending-stat-changes-bound.md) | ~~`pendingStatChanges` unbounded — missed turn boundary leaks cross-turn grouping~~ **DONE** — capped at 200 with early flush |
| 06 | [06-board-snapshot-restore.md](06-board-snapshot-restore.md) | ~~Board snapshot/restore unconditional — may restore combat-copy base stats on game over~~ **DONE** — snapshot gated to recruit phase |
| 07 | [07-parser-processor-coupling.md](07-parser-processor-coupling.md) | ~~Parser→Processor channel undocumented/unbuffered — processor block stalls log tail~~ **DONE** — buffered at 512 with non-blocking send |
| 08 | [08-local-player-name-match.md](08-local-player-name-match.md) | ~~`isLocalPlayerEntity` name-string match can false-positive — wrong stat attribution~~ **DONE** — name demoted to last resort |
| 09 | [09-cat-lightfang-consumed-dnt.md](09-cat-lightfang-consumed-dnt.md) | ~~`CatLightfang`/`CatConsumed` have no Dnt handlers — counters stuck at 0~~ **DONE** — confirmed no Dnt counters exist; per-minion only |
| 35 | [35-max-health-from-hero.md](35-max-health-from-hero.md) | ~~Max health hardcoded to 40 in TUI~~ **DONE** — MaxHealth tracked from hero HEALTH tag |

## MEDIUM

| # | File | Issue |
|---|------|-------|
| 10 | [10-opponent-tracking.md](10-opponent-tracking.md) | No opponent tracking — `BGGameState.Opponent`/`OpponentBoard` never populated |
| 11 | [11-stat-mod-source.md](11-stat-mod-source.md) | `Modifications[]` Source/Category/CardID always empty — block context not used |
| 12 | [12-win-loss-streak.md](12-win-loss-streak.md) | ~~`WinStreak`/`LossStreak` declared but never set~~ **DONE** — tracked via PREDAMAGE/TURN |
| 13 | [13-gold-tracking.md](13-gold-tracking.md) | ~~`CurrentGold` declared but never set~~ **DONE** — tracked via RESOURCES/RESOURCES_USED tags |
| 33 | [33-loss-streak-overcounting.md](33-loss-streak-overcounting.md) | **DONE** — Fixed: use PREDAMAGE tag instead of armor decrease to detect combat losses |
| 34 | [34-hero-damage-tracking.md](34-hero-damage-tracking.md) | Hero DAMAGE tag not tracked — health never updates (effective HP = HEALTH - DAMAGE) |
| 35 | [35-max-health-from-hero.md](35-max-health-from-hero.md) | ~~Max health hardcoded to 40 in TUI~~ **DONE** — MaxHealth tracked from hero HEALTH tag |
| 14 | [14-parser-panic-recovery.md](14-parser-panic-recovery.md) | ~~No panic recovery in `Feed()`~~ **DONE** — defer recover() with slog.Error |
| 15 | [15-dead-event-constants.md](15-dead-event-constants.md) | ~~`EventPlayerUpdate`/`EventZoneChange` declared but never emitted~~ **DONE** — removed dead constants |
| 16 | [16-timestamp-date.md](16-timestamp-date.md) | Timestamp uses today's date — reparse of old logs assigns wrong date; midnight wrap |
| 17 | [17-enchantment-table-staleness.md](17-enchantment-table-staleness.md) | ~~`categories.go` CardID map manually curated~~ **DONE** — runtime slog.Debug warning for untracked Dnt enchantments |

## LOW

| # | File | Issue |
|---|------|-------|
| 18 | [18-block-indent-threshold.md](18-block-indent-threshold.md) | `reBlockTag` hard-codes 4-space indent — breaks if Blizzard changes indentation |
| 19 | [19-retagchange-ordering.md](19-retagchange-ordering.md) | ~~`reTagChange` catch-all undocumented priority~~ **DONE** — ordering comment added |
| 20 | [20-block-type-parsing.md](20-block-type-parsing.md) | `BLOCK_START` `BlockType` ignored — can't distinguish attack/spell/play blocks |
| 21 | [21-zone-position.md](21-zone-position.md) | `ZONE_POSITION` tag ignored — board order and position-dependent buffs wrong |
| 22 | [22-combat-damage-tags.md](22-combat-damage-tags.md) | No `DAMAGED`/`DEFENDING`/`ATTACKING` handling — combat buff suppression heuristic fragile |
| 23 | [23-gameid-stable.md](23-gameid-stable.md) | `gameSeq` resets on restart — ID collisions in store after daemon restart |
| 24 | [24-reparse-seq-reset.md](24-reparse-seq-reset.md) | Reparse does not reset `gameSeq` — inconsistent IDs during live reparse |
| 25 | [25-snapshot-isolation.md](25-snapshot-isolation.md) | No historical board state query — all state is live and mutable |
| 26 | [26-integration-test-coverage.md](26-integration-test-coverage.md) | Integration tests cover only one log — edge cases untested |
| 27 | [27-rest-deep-copy.md](27-rest-deep-copy.md) | `machine.State()` deep-copies on every poll — GC pressure for polling clients |
| 28 | [28-game-history-pagination.md](28-game-history-pagination.md) | `/v1/stats/games` returns all games — unbounded response as store grows |
| 29 | [29-ws-sse-delta.md](29-ws-sse-delta.md) | WS/SSE broadcast full state on every event — high-frequency combat floods clients |
| 30 | [30-log-verbosity.md](30-log-verbosity.md) | Log verbosity not configurable at runtime — noise requires recompile to reduce |
| 31 | [31-metrics.md](31-metrics.md) | No metrics/observability — no Prometheus or similar export |
| 32 | [32-trinkets-artifacts.md](32-trinkets-artifacts.md) | No trinkets/artifacts support — post-2025 mechanics not covered |
| 36 | [36-placement-in-result.md](36-placement-in-result.md) | ~~TUI shows WIN/LOSS without placement number~~ **DONE** — displays "WIN #4" / "LOSS #7" |
