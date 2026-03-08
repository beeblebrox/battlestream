# 33 — Loss streak keeps incrementing even on wins

Priority: **HIGH**
Status: **DONE**

## Bug

The loss streak counter was called on every armor decrease rather than once per
combat round, causing overcounting. Additionally, non-combat hero damage (e.g.
Imposing Percussionist self-damage) was counted as combat losses.

## Root cause

`RecordRoundLoss()` was called eagerly inside the ARMOR TAG_CHANGE handler —
every armor decrease triggered it immediately. This caused:
1. Multiple loss increments per round (multiple damage events in one combat).
2. Non-combat armor changes (hero powers, card effects) counted as losses.

Previous fix attempts:
- **PREDAMAGE tag**: fires on ALL hero damage including self-damage from cards
  like Imposing Percussionist, not just combat losses. Wrong.
- **Armor snapshot**: armor changes arrive after combat phase timing, unreliable.

## Fix

Used `PROPOSED_ATTACKER` on `GameEntity` (HDT's `LastAttackingHero` pattern).

In BG, after minion combat ends, the winning side's hero attacks the losing
hero for damage. `TAG_CHANGE Entity=GameEntity tag=PROPOSED_ATTACKER value=<id>`
fires for every attack during combat. We only track hero entity attackers (via
the `heroEntities` registry), ignoring minion attacks.

At each player TURN boundary:
- If `lastCombatHeroAttackerID == localHeroID` → WIN (local hero attacked = won)
- If `lastCombatHeroAttackerID > 0` (other hero) → LOSS (opponent won)
- If `lastCombatHeroAttackerID == 0` → TIE (no hero attacked, streak preserved)

Changes in `processor.go`:
- Added `PROPOSED_ATTACKER` case in `handleTagChange`: when `Entity=GameEntity`
  and the value is a known hero entity ID, stores it as `lastCombatHeroAttackerID`.
- At player TURN boundary: evaluates win/loss/tie from the hero attacker ID.
- Removed old PREDAMAGE-based detection.

Verified against the 2026-03-07 game log:
- Rounds 1-2: LOSS (opponent hero attacks local hero)
- Rounds 3-6: WIN (local hero attacks)
- Round 7: TIE (no hero attack, streak preserved at W=3)
- Rounds 8-10: WIN
- Round 11: TIE (streak preserved at W=6)
- Rounds 12-15: WIN
- Round 13: correctly WIN — armor drop (8→3) is from Imposing Percussionist, not combat
- Round 16: not recorded (game ends before next TURN fires)
- Final: WinStreak=11, LossStreak=0 ✓
