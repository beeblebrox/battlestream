---
name: BG35 set status
description: As of 2026-05-08, the BG35 set ("Beasts of the Deep" era) is absent from both HearthDb and HDT
type: project
---

As of 2026-05-08, neither HearthDb (`reference/HearthDb/HearthDb/`) nor HDT
(`reference/Hearthstone-Deck-Tracker/`) contains any BG35_ CardID constants
or counter classes.

**Why:** The reference repos are vendored snapshots and have not yet been
updated for the latest BG patch.

**How to apply:** Before researching any BG35_xxx card, run `git pull` in
both reference repos. If still missing, the card cannot be confidently
implemented (CardID suffixes like `pe`/`e`/`e2` are not predictable, and
HDT's HandleTagChange logic is the source of truth for accumulation patterns).
