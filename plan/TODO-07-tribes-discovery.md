# TODO-07: Investigate Earlier Tribe Discovery

Status: **DONE** (investigation complete + accuracy fix applied)
Priority: **MEDIUM**

## Problem

The current implementation discovers available tribes by collecting `BACON_SUBSET_*`
tags as they appear on individual pool minions. The game client shows available
tribes before hero selection — could we detect them earlier?

## Investigation results

### No early tribe tag exists in Power.log

Searched the test log (`power_log_2026_03_07.txt`) — the first `BACON_SUBSET_*`
and `CARDRACE` tags appear at line ~2237, well after `TURN=1` (line 1861). No
tribe/race/banned/available tags appear in the CREATE_GAME block or before TURN=1.

### HDT uses memory reflection, not log parsing

HDT detects available tribes via `Reflection.Client.GetAvailableBattlegroundsRaces()`
— a live memory read from the Hearthstone process. This is **not** available through
Power.log parsing.

Reference: `reference/Hearthstone-Deck-Tracker/Hearthstone Deck Tracker/Hearthstone/BattlegroundsUtils.cs` line 66.

### HearthDb tag catalog

- `CARDRACE` (GameTag 200) — per-minion primary race
- `BACON_SUBSET_*` (GameTags 1591-1596, 1688, 1845, 2272, 2347) — per-minion tribe flags
- No `BACON_AVAILABLE_RACES` or `BACON_BANNED_RACE` GameTag exists in HearthDb enums

### Conclusion

To show tribes before hero selection would require memory reflection, which is
out of scope for a log-based parser. Tribes populate as soon as shop minions
appear (early turn 1).

## Bug fix: multi-tribe minion bleeding

The naive approach of collecting ALL entities with any `BACON_SUBSET_*` tag was
**wrong** — it detected banned tribes as available because multi-tribe minions
(e.g., a Dragon/Demon dual-type like BG32_821) appear in the pool via their
available tribe but also carry `BACON_SUBSET_*` tags for their banned tribe.

### Root cause

In BG, multi-tribe minions are in the pool if ANY of their tribes is available.
A Dragon/Demon minion enters the pool because Demons are available, but it also
has `BACON_SUBSET_DRAGON=1`, causing Dragons to be falsely detected as available.

### Fix

Changed `trackTribesFromEntity()` to only register a tribe from entities with
**exactly one** `BACON_SUBSET_*` tag (single-tribe minions). Multi-tribe and
Amalgam (ALL) entities are excluded.

Verified against 2026-03-07 log:
- **Single-tribe entities:** DEMON=267, MURLOC=135, BEAST=126, QUILBOAR=125, PIRATE=109
- **Multi-tribe entities:** DRAGON=27, ELEMENTALS=25, NAGA=8, UNDEAD=4
- Result: correctly detects 5 available tribes, excludes 4 banned tribes

## Follow-up fixes

### Missing MECH tribe suffix

`baconSubsetToTribe` map was missing `"MECH": "Mech"`. The 2026-03-08 log uses
`BACON_SUBSET_MECH` (not `BACON_SUBSET_MECHANICAL`). All 10 BG tribe suffixes
now mapped: BEAST, DEMON, DRAGON, ELEMENTALS, MECH, MURLOC, NAGA, PIRATE,
QUILLBOAR, UNDEAD.

### TAG_CHANGE tribe detection with multi-tribe revocation

Added `BACON_SUBSET_*` handling in `handleTagChange` `default` case for faster
tribe detection (TAG_CHANGE events arrive before SHOW_ENTITY blocks). Because
TAG_CHANGE events arrive individually, multi-tribe minions fire separate events.
Solution: per-entity subset count with confirmation tracking — provisionally
register on first subset, revoke if a second arrives (entity is multi-tribe).
Uses `tribeConfirmCount` map so revocation only removes a tribe if no other
single-tribe entity confirms it.
