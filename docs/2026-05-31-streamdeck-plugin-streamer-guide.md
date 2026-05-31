# Battlestream Stream Deck Plugin — Streamer Guide

**Who this is for:** streamers and players setting up the plugin on their Stream Deck. No technical background needed. If you want the precise engineering details, see the companion **[Functional Specification](2026-05-31-streamdeck-plugin-spec.md)**.

---

## Contents

1. [Quick Start](#1-quick-start)
2. [The Two Kinds of Button](#2-the-two-kinds-of-button)
3. [Individual Buttons — Pin What You Always Want](#3-individual-buttons--pin-what-you-always-want)
4. [The Dyn Button — Auto-Filling Buff Keys](#4-the-dyn-button--auto-filling-buff-keys)
   - 4.1 [What it's for](#41-what-its-for)
   - 4.2 [It's still labeled "Buff Slot" for now](#42-its-still-labeled-buff-slot-for-now)
   - 4.3 [Placing several, and the fill order](#43-placing-several-and-the-fill-order)
   - 4.4 [How they change over a game (worked example)](#44-how-they-change-over-a-game-worked-example)
   - 4.5 [What you'll see on a key](#45-what-youll-see-on-a-key)
5. [Pin It or Use a Dyn Button?](#5-pin-it-or-use-a-dyn-button)
6. [Troubleshooting](#6-troubleshooting)

---

## 1. Quick Start

If you just want your deck working, this is all you need:

- **Start your tracker first.** The plugin shows *live* data read from the Battlestream tracker (the "daemon"). Make sure it is running before you expect numbers. **If every key shows a dash `—`, the tracker isn't reachable** — start it and check your connection settings.
- **Set up the connection once.** Open the plugin's global settings (the gear / property inspector) and enter the host, port, and (if you use one) API key for your tracker. This applies to every button.
- **Drag on the stats you always want.** Health, gold, and tavern tier appear in *every* game, so each one is its own button. Drop them wherever you like and they stay put.
- **For buffs that only show up some games, use "Dyn Buttons."** Beetles, Whelps, Spellcrafts, and similar buffs depend on your tribes and what you buy — they aren't in every game. Instead of dedicating a key to each one, drop **a few Dyn Buttons** on your deck and the plugin fills them automatically with whatever buffs are active right now, swapping them as the game changes. **Heads-up:** until the rename ships, this button is still labeled **"Buff Slot"** in the Stream Deck action list — that is the one to drag on.
- **More Dyn Buttons = more buffs visible at once.** In a typical game only 2–4 buff types are active at once (your main synergy plus a couple of incidental ones), so 2–4 Dyn Buttons usually shows everything. If you place two and three buffs become active, only two show; the third waits until a slot frees up or replaces one of the others.
- **A blank black Dyn Button is normal** — it just means no extra buff is assigned to that slot yet. It is *not* an error. The unambiguous trouble signal is a **dash `—`**: any key showing a dash can't reach your tracker.

---

## 2. The Two Kinds of Button

The plugin gives you **two fundamentally different kinds of button**:

| | **Individual button** | **Dyn Button** |
|---|---|---|
| What it shows | Fixed — always the same stat | Auto-fills with whatever buff is active |
| How you place it | One key = one fixed metric | Place as many as you like; they share a pool |
| How many are in the action list | Many (one per stat/buff) | Exactly one |
| When the tracker is unreachable | Shows `—` / `OFFLINE` | Goes blank (plain black) |

> **Mental model:** think of individual buttons as **reserved parking** — one car, one spot, always the same car. Dyn Buttons are a **shared lot** that the active buffs pull into and out of as the game goes.

The two kinds are **complementary** — you can use both at once. You can pin Bloodgems to a fixed key *and* still have it show up in the Dyn pool; they don't interfere.

---

## 3. Individual Buttons — Pin What You Always Want

An individual button always shows **one fixed thing**. You decide which key shows which stat, and it never changes mid-game. They're perfect for the metrics that exist in *every* game on *every* turn.

What you can pin individually:

- **Core stats:** Health, Armor, Tavern Tier, Gold, Triples, Win Streak, Loss Streak, Placement, Turn, Phase, Minion Count, Anomaly.
- **A specific buff you always care about:** there's a dedicated button for **every** buff type — Bloodgems, Elementals, Tavern-Wide, BG Barrage, Rightmost, Nomi, Undead, Lightfang, Whelps, Beetles, Volumizer, Consumed.
- **Counters:** Minions Sold, Spells Cast, Spellcraft Cast, Spellcraft (Naga stacks).
- **Opponent / summary:** Opponent Health, Opponent Tavern Tier, Available Tribes, Total Buffs.

> **Good to know:** every buff that can appear in a Dyn Button *also* has its own individual button. So if there's a buff you never want to miss, you can pin it to a fixed key — and it can still appear in your Dyn pool too.

---

## 4. The Dyn Button — Auto-Filling Buff Keys

### 4.1 What it's for

Lots of buffs and counters only show up in **some** games:

- **Tribe-gated** — e.g. Whelps (only when Dragons are in the lobby).
- **Card/mechanic-gated** — e.g. Bloodgem Barrage, Beetles (Hunter Beetle payoffs), Spellcraft (Nagas) — only when you pick up the relevant cards.
- **Anomaly-gated** — certain effects only appear under specific anomalies.

You *could* pin a fixed key for every one of these, but that's a lot of keys for buffs you might not even see this game. Instead, drop **a few Dyn Buttons**. The plugin fills them with whichever buffs are currently active and **rotates** them as the game evolves — so you reserve a handful of keys instead of dozens.

### 4.2 It's still labeled "Buff Slot" for now

This guide calls the auto-filling key the **Dyn Button** (short for "Dynamic"). In the current Stream Deck action list it is still labeled **"Buff Slot"** — that's the one to drag on until the rename ships.

> **For existing users:** if you already placed "Buff Slot" keys, they keep working unchanged after the rename. Only the *name* in the action list changes — you don't need to re-add anything.

### 4.3 Placing several, and the fill order

You can place the Dyn Button on **as many keys as you like**. Active buffs fill them in **reading order**: top row left-to-right, then the next row down. Even if you scatter Dyn Buttons around your deck, the highest, leftmost one fills first.

Imagine a 3×3 deck with Dyn Buttons (`D`) at three spots and other buttons (`·`) elsewhere. The numbers show the order active buffs fill them:

```
 col:  0     1     2
row 0 [D1]  [·]   [D2]
row 1 [·]   [D3]  [·]
row 2 [·]   [·]   [·]
```

The first active buff lands on the top-left Dyn Button (`D1`), the second on `D2`, the third on `D3` — regardless of where the non-Dyn buttons sit.

**The key tradeoff:** the number of Dyn Buttons you place is your dial.

- **More Dyn Buttons** → more situational buffs visible at the same time.
- **Fewer Dyn Buttons** → when more buffs are active than you have keys, some get replaced and only the survivors stay visible.

### 4.4 How they change over a game (worked example)

Say you placed **2** Dyn Buttons — a **left/top** one and a **right/lower** one (the left/top one fills first because it's on a higher row).

| Game moment | Active buffs | Your two keys (left-top / right-lower) | What happened |
|---|---|---|---|
| Early, nothing bought | (none) | `[ (blank) ] [ (blank) ]` | Nothing active yet. |
| Buy a bloodgem minion | Bloodgems | `[ Bloodgems +4/+6 ] [ (blank) ]` | First buff fills the first key. |
| Beetles come online | Bloodgems, Beetles | `[ Bloodgems +4/+6 ] [ Beetles +2/+2 ]` | Second buff fills the second key. |
| Whelps come online — **both keys full** | Bloodgems, Beetles, Whelps | `[ Whelps +3/+3 ] [ Beetles +2/+2 ]` | Both keys are full, so one is reused for Whelps. With only 2 Dyn Buttons you can't see all three — **add a 3rd** and all three would show. |
| Buffs grow but stay active | all stay active | numbers refresh, e.g. `[ Whelps +5/+5 ] [ Beetles +4/+4 ]` | Active keys just update their numbers. |
| Beetles drop to `+0/+0` | Beetles gone | `[ Whelps +5/+5 ] [ (blank) ]` | The Beetles key goes blank — that buff is no longer active. |
| Tracker stops / game ends | — | `[ (blank) ] [ (blank) ]` | All Dyn Buttons clear. |

> **Don't count on *which* key gets reused.** When all your Dyn Buttons are full and a new buff appears, the plugin reuses one of them — but **which one is not guaranteed to follow position**. It depends on the order buffs came and went during the game, so it can look arbitrary. If you don't want any buff dropped, just **add more Dyn Buttons.**

### 4.5 What you'll see on a key

> **At a glance:** a **dash `—`** means a connection problem; a **blank black key** means an unused slot. A blank Dyn Button is never an "error" tile.

| What you see | What it means | What to do |
|---|---|---|
| **Plain black, no text** (a Dyn Button) | No buff is assigned to this slot yet | Nothing — this is normal |
| **Colored, with a buff name on top and numbers below** | A buff is active and assigned here | Nothing |
| **A dash `—`** (greyed-out key) | The plugin can't reach your tracker | Check the tracker is running and your connection settings |

---

## 5. Pin It or Use a Dyn Button?

| Your situation | Recommendation |
|---|---|
| A stat you want **every game** (health, gold, tavern tier, turn, placement) | **Pin it** — individual button |
| A **specific buff you always care about** (e.g. Bloodgems, Elementals) | **Pin it** — its individual buff button |
| Buffs that **only show up some games** (Beetles, Whelps, Spellcrafts, Barrage…) | **Use Dyn Buttons** |
| You have **only a few free keys** | **Use Dyn Buttons** |

**Headline tradeoff:** *Pinning* = always there, but one key per buff. *Dyn Buttons* = auto-fill themselves, but only show as many buffs at once as the number you placed.

**Rule of thumb:** in a typical game only **2–4 buff types are active at once** (your main synergy plus a couple of incidental ones), which is why **2–4 Dyn Buttons** usually shows everything without dropping anything. Watch a few of your own runs — if you keep seeing a buff get replaced and wish it had stayed, add one more Dyn Button.

---

## 6. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| **Every key shows `—`** | The plugin can't reach your tracker | Start the Battlestream tracker (daemon); check host/port/API key in the plugin's global settings |
| **A Dyn Button is blank black** | No buff assigned to that slot yet | Normal — nothing to do |
| **A buff I expected isn't showing on a Dyn Button** | More buffs are active than you have Dyn Buttons, so it was replaced | Add more Dyn Buttons |
| **I can't find the "Dyn Button" in the action list** | It's still labeled **"Buff Slot"** until the rename ships | Drag on **"Buff Slot"** |
| **Numbers look frozen** | Tracker stopped sending updates | Confirm the tracker is still running; the plugin re-polls automatically and reconnects on its own |

---

*For the full technical behavior (rendering, the assignment algorithm, every button's exact value source, and known open questions), see the **[Functional Specification](2026-05-31-streamdeck-plugin-spec.md)** and the **[Ambiguities & Open Questions](2026-05-31-streamdeck-plugin-spec-ambiguities.md)**.*
