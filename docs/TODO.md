# TODO

> **This file is a thin pointer. [`plan/README.md`](../plan/README.md) is the
> authoritative issue tracker** — it has per-item detail files, priorities, and
> done/open status. This file only summarizes what is still genuinely open as of
> the 2026-06-10 audit ([`docs/audit-2026-06-10.md`](audit-2026-06-10.md)).
> Each item below was re-verified against the code on this date.

## Still open

| Plan item | Issue | Verified status |
|---|---|---|
| [11](../plan/11-stat-mod-source.md) | Board-wide `StatMod` entries leave `Source`/`Category`/`CardID` empty | Open — `flushPendingStatChanges` fills only `Turn`/`Target`/`Stat`/`Delta`; block context (`BlockSource`/`BlockCardID`) unused |
| [20](../plan/20-block-type-parsing.md) | `BLOCK_START` `BlockType` not captured | Open — `reBlockStart` matches `BlockType=\w+` but does not capture the value |
| [22](../plan/22-combat-damage-tags.md) | No `DAMAGED`/`DEFENDING`/`ATTACKING` tag handling | Open — combat damage vs. buff distinction still relies on the phase heuristic |
| [27](../plan/27-rest-deep-copy.md) | REST `/v1/game/current` deep-copies full state on every poll | Open — `machine.State()` copies all slices per read; GC pressure for polling clients |
| [29](../plan/29-ws-sse-delta.md) | WS/SSE flood clients during high-frequency phases | Open, but reframed: the server broadcasts **every parsed `GameEvent`** (not, as previously claimed, a full `BGGameState` snapshot per event); no batching/filtering/delta protocol |
| [28](../plan/28-game-history-pagination.md) | No cursor-based pagination on `/v1/stats/games` | Partially done — `limit` (default 50) / `offset` / `mode` params shipped; cursor pagination still absent |
| [31](../plan/31-metrics.md) | No metrics/observability | Open — no Prometheus or similar export |
| [25](../plan/25-snapshot-isolation.md) | Historical board state queries | Partially done — per-turn snapshots are persisted and queryable via `GET /v1/game/{id}/turns`; live in-memory state remains mutable-only |
| [32](../plan/32-trinkets-artifacts.md) | No trinkets/artifacts support | Open — no trinket coverage in `categories.go` |
| [26](../plan/26-integration-test-coverage.md) | Integration test coverage | Partially done — multiple full-game logs now exercised (2026-03-07, 2026-05-08, duos fixtures), but named edge cases (Nomi All, Timewarped, game ending mid-combat) remain untested |

## Everything else

All other items from previous versions of this file were verified **done** in
code and are struck through with notes in [`plan/README.md`](../plan/README.md).
Highlights: dead event constants removed; parser state reset on game start;
midnight timestamp wrap fixed (`refDate`); `parseInt` uses `strconv.Atoi`;
`pendingStatChanges` capped; stable timestamp-based game IDs; win/loss streak,
gold, and hero damage tracking shipped; `CatSpellcraft` → `CatNagaSpells`
rename; `CatLightfang`/`CatConsumed` confirmed per-minion only; panic recovery
in `Feed()`; parser→processor channel buffered; log verbosity configurable.

Opponent tracking (plan item 10) is **partially** done: `OpponentBoard` is
populated from combat copies, but the `Opponent` hero `PlayerState` is still
never set — see `plan/README.md` for status.

Newly found bugs and risks live in the audit report, not here:
[`docs/audit-2026-06-10.md`](audit-2026-06-10.md).
