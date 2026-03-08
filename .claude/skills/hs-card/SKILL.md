---
name: hs-card
description: Look up current Hearthstone card data by name or ID. Supports all cards and Battlegrounds-specific filtering. Use when the user asks about a Hearthstone card's stats, text, tier, mechanics, or wants to search for cards by name.
---

# Hearthstone Card Lookup

Look up Hearthstone card(s): **$ARGUMENTS**

## Script

All lookups are done via [hs_card.py](hs_card.py). Run it with:

```bash
.claude/skills/hs-card/hs_card.py [name...] [flags]
```

## Flags

| Flag | Values | Description |
|------|--------|-------------|
| `name` | any text | Partial card name, case-insensitive |
| `--bg` | — | Battlegrounds cards only (implied by BG-specific flags) |
| `--tier N` | 1–7 | BG tech level |
| `--tribe NAME` | all, beast, demon, dragon, elemental, mechanical, murloc, naga, pirate, quilboar, undead | Filter by tribe (implies --bg) |
| `--type TYPE` | minion, spell, battleground_spell, battleground_trinket | Filter by card type |
| `--class CLASS` | deathknight, demonhunter, druid, hunter, mage, neutral, paladin, rogue, shaman, warlock, warrior | Filter by hero class |
| `--rarity RARITY` | common, epic, free, legendary, rare | Filter by rarity |
| `--keyword KEYWORD` | aura, avenge, battlecry, choose_one, deathrattle, discover, divine_shield, end_of_turn_trigger, magnetic, poisonous, reborn, start_of_combat, stealth, taunt, venomous, windfury | Filter by mechanic/keyword |
| `--buddy` | — | BG buddy minions only (implies --bg) |
| `--pool` | — | BG pool cards only (implies --bg) |
| `--image` | — | Include card image in output |
| `--id ID` | card ID | Exact card ID lookup |
| `--limit N` | number | Max results shown (default 10) |

## Instructions

If `$ARGUMENTS` is empty, reply with this usage summary and stop — do not run the script:

```
Usage: /hs-card [name] [flags]

Flags:
  --bg              Battlegrounds cards only
  --tier N          BG tech level 1-7
  --tribe NAME      Tribe: all, beast, demon, dragon, elemental, mechanical,
                           murloc, naga, pirate, quilboar, undead
  --type TYPE       Card type: minion, spell, battleground_spell, battleground_trinket
  --class CLASS     Class: deathknight, demonhunter, druid, hunter, mage, neutral,
                           paladin, rogue, shaman, warlock, warrior
  --rarity RARITY   Rarity: common, epic, free, legendary, rare
  --keyword KW      Keyword: aura, avenge, battlecry, deathrattle, discover,
                             divine_shield, magnetic, poisonous, reborn,
                             start_of_combat, stealth, taunt, venomous, windfury
  --buddy           BG buddy minions only
  --pool            BG pool cards only
  --image           Include card image in output
  --id ID           Exact card ID
  --limit N         Max results (default 10)

Examples:
  /hs-card murloc warleader
  /hs-card --bg --tier 7
  /hs-card --tribe quilboar --tier 7
  /hs-card --tribe murloc --keyword divine_shield
  /hs-card --class neutral --keyword taunt --tier 3
  /hs-card --buddy
  /hs-card --pool --tribe beast
  /hs-card --id BG25_034
```

Otherwise, parse `$ARGUMENTS` into the correct flags using these rules:
- Tribe names → `--tribe NAME` (e.g. "quilboar", "murlocs" → quilboar/murloc)
- "tier N" or "T N" → `--tier N`
- Keyword/mechanic names → `--keyword NAME` (e.g. "divine shield" → divine_shield, "deathrattle" → deathrattle)
- Class names → `--class NAME`
- "buddies" / "buddy" → `--buddy`
- "pool" / "in the pool" → `--pool`
- "legendary" / "epic" etc. → `--rarity`
- "spell" / "minion" → `--type`
- Any other text is a name search
- BG-context words ("battlegrounds", "bg") → `--bg`
- "image" / "show image" / "picture" / "art" → `--image`

Then run the script. Show the output directly — it is already formatted for display. Exception: if the output contains one or more lines starting with `Image: /`, extract each local file path and run `xdg-open <path>` via Bash to open it in the system image viewer. Do not use the Read tool for images — it only sends them to Claude's context, not the user's screen.

## Examples

```
/hs-card murloc warleader                       → .claude/skills/hs-card/hs_card.py "murloc warleader"
/hs-card ragnaros --bg                          → .claude/skills/hs-card/hs_card.py ragnaros --bg
/hs-card --bg --tier 4                          → .claude/skills/hs-card/hs_card.py --bg --tier 4
/hs-card --tribe quilboar --tier 7              → .claude/skills/hs-card/hs_card.py --tribe quilboar --tier 7
/hs-card --tribe murloc --keyword divine_shield → .claude/skills/hs-card/hs_card.py --tribe murloc --keyword divine_shield
/hs-card --class neutral --keyword taunt        → .claude/skills/hs-card/hs_card.py --class neutral --keyword taunt
/hs-card --buddy                                → .claude/skills/hs-card/hs_card.py --buddy
/hs-card --pool --tribe beast                   → .claude/skills/hs-card/hs_card.py --pool --tribe beast
/hs-card --rarity legendary --bg                → .claude/skills/hs-card/hs_card.py --rarity legendary --bg
/hs-card --id BG_EX1_507                        → .claude/skills/hs-card/hs_card.py --id BG_EX1_507
/hs-card --bg --tier 2 --limit 20               → .claude/skills/hs-card/hs_card.py --bg --tier 2 --limit 20
```
