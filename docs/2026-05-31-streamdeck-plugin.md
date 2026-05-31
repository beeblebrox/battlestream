# Battlestream Stream Deck Plugin — Overview & Index

The Battlestream Stream Deck plugin surfaces **live Hearthstone Battlegrounds statistics** on an Elgato Stream Deck. It connects to the local Battlestream tracker (daemon) and renders one game metric per key.

This is the **top-level index**. The documentation is split into three documents by audience and purpose — start with the one that fits you.

---

## Pick your document

| Document | Read it if you are… | What's inside |
|---|---|---|
| **[Streamer Guide](2026-05-31-streamdeck-plugin-streamer-guide.md)** | A streamer/player setting up your deck | Plain-language setup, no code. Quick start, the two kinds of button, how the auto-filling "Dyn Button" works, and a pin-it-or-not decision table. |
| **[Functional Specification](2026-05-31-streamdeck-plugin-spec.md)** | A developer or power user | The precise, source-grounded as-built spec: data flow, rendering, every button's exact value source, the Dyn Button assignment algorithm, profiles, tests, and appendices. |
| **[Ambiguities & Open Questions](2026-05-31-streamdeck-plugin-spec-ambiguities.md)** | Deciding what to build/fix next | Decisions deliberately deferred to a human — the Dyn Button rename scope, the eviction-policy behavior question, the `SPELL PWR` label misnomer, display-name drift, and more. |

---

## The plugin in one minute

There are **two kinds of button**:

| | **Individual button** | **Dyn Button** |
|---|---|---|
| Shows | A fixed stat, always the same | Auto-fills with whatever buff is active |
| Placement | One key = one metric | Place as many as you like; they share a pool |
| Count in the action list | Many (one per stat/buff) | Exactly one |
| Tracker unreachable | `—` / `OFFLINE` | Blank black key |

- **Individual buttons** are for the things you want *every* game (health, gold, tavern tier) and for any specific buff you want permanently pinned. Each is its own action with its own icon.
- **The Dyn Button** is for the *situational* buffs that aren't in every game (Beetles, Whelps, Spellcrafts, …). Place a few; the plugin fills them with the currently-active buffs and rotates them as the game evolves.
- **Both forms always exist:** every buff that can appear in a Dyn Button also has its own individual button — so you can pin it, let it flow through the Dyn pool, or both.

> **Naming note:** the auto-filling key is referred to as the **"Dyn Button"** across these docs (a planned rename). In the current build it is still labeled **"Buff Slot"** in the Stream Deck action list. Existing placements keep working after the rename.

---

## Related (historical) design docs

The original design notes under `docs/superpowers/specs/` (`2026-05-05-streamdeck-plugin-design.md`, `2026-05-08-buff-grouping-design.md`) predate the current implementation and are **partly stale** (they reference removed features such as Auto-Layout and the old `buff-atk`/`buff-hp` actions). The three documents linked above supersede them for current behavior.
