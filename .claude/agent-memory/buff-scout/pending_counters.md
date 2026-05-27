---
name: Pending counters awaiting reference data
description: Cards identified as candidates for buff-source tracking but blocked on HDT/HearthDb/Power.log evidence
type: project
---

Counters we want to add but cannot safely implement yet:

## Lurking Leviathan (BG35_602)
- Beast +ATK summon-stacker, similar shape to Whelp/Beetle
- Enchantment expected to be `BG35_602e` ("Leviathan's Wrath") — suffix unconfirmed (could be `pe` like Whelp, or `e2`)
- Likely player-level Dnt with TAG_SCRIPT_DATA_NUM_1 only (ATK-only, no HP component)
- Closest HDT analog: `WhelpStatsBuffCounter` (single-NUM_1 absolute) or `UndeadAttackBonusCounter` (SD1-only)
- Proposed category constant: `CatLeviathan = "LEVIATHAN"`, group `GroupTypeBuffs`

**Why blocked:** BG35 absent from HearthDb + HDT; no Power.log capture exists with this card.

**How to apply:** Before implementing, (1) `git pull` both reference repos, (2) verify the CardID suffix, (3) capture a real Power.log of a game using Lurking Leviathan and confirm whether the running total lives on a player-Dnt or on the Lurking Leviathan minion entity itself.
