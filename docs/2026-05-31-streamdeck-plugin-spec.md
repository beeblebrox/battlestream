# Battlestream Stream Deck Plugin — Functional Specification

**Document type:** Implementation specification (as-built)
**Scope:** Documents the plugin *as it currently exists* in `/chungus/projects/battlestream/streamdeck-plugin`. No code changes are proposed by this document. The single forward-looking rename it records (the dynamic button's display name) is called out explicitly and tracked in the companion ambiguities list.
**Manifest version documented:** `1.1.0.0` (`manifest.json`)
**Plugin UUID:** `com.battlestream.streamdeck`

> **Companion documents**
> - **[Stream Deck Plugin — Overview & Index](2026-05-31-streamdeck-plugin.md)** — start here; routes each audience to the right doc.
> - **[Streamer Guide](2026-05-31-streamdeck-plugin-streamer-guide.md)** — plain-language setup for streamers (no code).
> - **[Ambiguities & Open Questions](2026-05-31-streamdeck-plugin-spec-ambiguities.md)** — decisions deferred to a human.
>
> *This document is the precise, source-grounded technical specification for developers and power users.*

---

## Table of Contents

1. [For Streamers — Quick Start](#1-for-streamers--quick-start)
2. [Purpose and Audience](#2-purpose-and-audience)
3. [Terminology](#3-terminology)
4. [System Overview](#4-system-overview)
   - 4.1 [Runtime and Tooling](#41-runtime-and-tooling)
   - 4.2 [Data Source and Client](#42-data-source-and-client)
   - 4.3 [State Store and Subscription Lifecycle](#43-state-store-and-subscription-lifecycle)
   - 4.4 [GameState Shape](#44-gamestate-shape)
   - 4.5 [Rendering](#45-rendering)
   - 4.6 [Global Settings](#46-global-settings)
5. [The Two Kinds of Button](#5-the-two-kinds-of-button)
6. [Individual (Static) Buttons](#6-individual-static-buttons)
   - 6.1 [Why They Are Individual](#61-why-they-are-individual)
   - 6.2 [Core Stat Buttons](#62-core-stat-buttons)
   - 6.3 [Per-Category Buff Buttons](#63-per-category-buff-buttons)
   - 6.4 [Opponent and Summary Buttons](#64-opponent-and-summary-buttons)
   - 6.5 [The Three Spell Counters](#65-the-three-spell-counters)
7. [The Dynamic Button ("Dyn Button")](#7-the-dynamic-button-dyn-button)
   - 7.1 [Identity and Naming](#71-identity-and-naming)
   - 7.2 [Purpose](#72-purpose)
   - 7.3 [Eligible Categories](#73-eligible-categories)
   - 7.4 [What the Three Buff Groups Mean](#74-what-the-three-buff-groups-mean)
   - 7.5 [Active-Category Definition](#75-active-category-definition)
   - 7.6 [Placement and the Fill Region](#76-placement-and-the-fill-region)
   - 7.7 [Assignment Algorithm](#77-assignment-algorithm)
   - 7.8 [Rendering of Dyn Buttons — What You'll See](#78-rendering-of-dyn-buttons--what-youll-see)
   - 7.9 [Worked Example Over a Game](#79-worked-example-over-a-game)
8. [The Key Rule: Both Forms Always Exist](#8-the-key-rule-both-forms-always-exist)
9. [Why Both Kinds Exist (Design Intent)](#9-why-both-kinds-exist-design-intent)
   - 9.1 [Should I Pin It or Use a Dyn Button?](#91-should-i-pin-it-or-use-a-dyn-button)
10. [Bundled Profiles](#10-bundled-profiles)
11. [Test Coverage](#11-test-coverage)
12. [Appendix A: Complete Manifest Action List](#appendix-a-complete-manifest-action-list)
13. [Appendix B: CATEGORY_META Reference](#appendix-b-category_meta-reference)
14. [Open Questions](#open-questions)

---

## 1. For Streamers — Quick Start

> **Moved.** The plain-language setup walkthrough now lives in the **[Streamer Guide](2026-05-31-streamdeck-plugin-streamer-guide.md)**. If you just want your deck working, read that document instead — it requires no technical background. This section is retained only as an anchor for cross-references below.

The remainder of this document is the precise technical specification for developers and power users.

---

## 2. Purpose and Audience

The Battlestream Stream Deck plugin surfaces **live Hearthstone Battlegrounds (BG) statistics** on an Elgato Stream Deck. It connects to the local battlestream daemon's REST/SSE API and renders one game metric per key.

This specification has **two audiences**, each with its own document:

- **Streamers / non-technical users** — read the separate **[Streamer Guide](2026-05-31-streamdeck-plugin-streamer-guide.md)**, written in plain language with no code. The plain-language summaries here (marked **"What you'll see"** and **"Plain version"** in [§7](#7-the-dynamic-button-dyn-button)–[§9](#9-why-both-kinds-exist-design-intent)) are retained so this spec stays self-contained, but the Guide is the recommended starting point for non-technical readers.
- **Developers / power users** — this document: the full as-built detail. Source-grounded internals appear in **Implementation note** callouts and code snippets.

Every behavioral claim in this document is grounded in the plugin source (`streamdeck-plugin/src/**`) and `manifest.json`. Items that require a future human decision are **not** invented here; they are recorded in the companion *Ambiguities & Open Questions* list.

---

## 3. Terminology

| Term | Definition |
|------|------------|
| **Action** | A Stream Deck *action type* declared in `manifest.json` (`Actions[]`). Each has a unique `UUID`, a `Name`, and an icon. In the Stream Deck app this is one entry in the plugin's list that a user drags onto a key. |
| **Instance** | A single placement of an Action on a physical key. Stream Deck identifies each instance by a unique per-instance id, exposed in the SDK as **`action.id`**. One Action type may have many instances. |
| **Individual (Static) button** | An Action whose displayed metric is fixed at design time. Each concept (health, gold, a specific buff category, etc.) is its own Action with its own UUID and icon. Synonyms in source comments: *static button*, *core stat button*, *per-category button*. |
| **Dynamic button / "Dyn Button"** | The single Action (`…buff-slot`) whose displayed metric is **not** fixed; it is assigned at runtime from the set of currently-active situational categories. See [§7](#7-the-dynamic-button-dyn-button). |
| **Category** | A string key identifying a buff source or ability counter (e.g. `BLOODGEM`, `ELEMENTAL`, `MINIONS_SOLD`). Categories arrive from the daemon inside `buff_sources[]` and `ability_counters[]`. |
| **Buff source** | A `{category, attack, health}` record describing accumulated stat buffs of one category. Rendered as `+ATK/+HP`. |
| **Ability counter** | A `{category, value, display}` record describing a counted event (minions sold, spells cast, etc.). Rendered as a plain integer. |
| **Aggregate category** | A synthetic category whose value is the sum of several underlying categories. The only one is `TAVERN_WIDE` = `NOMI_ALL` + `SHOP_BUFF` + `GENERAL`. |
| **Active category** | A category currently carrying a non-zero value; see the precise definition in [§7.5](#75-active-category-definition). |
| **Slot** | A runtime binding of one Dyn Button instance (`action.id`) to one category, with a `lastUpdated` timestamp. Stored in `DynamicBuffSlotAction.slots` (a `Map`). |
| **Fill region** | The ordered collection of all currently-placed Dyn Button instances, sorted row-major, into which active categories are filled. |
| **Row-major order** | Sort by `row * 1000 + column` ascending — top-left key first, proceeding left-to-right then top-to-bottom. |
| **Eviction** | When every Dyn Button is occupied and a new category becomes active, an occupied slot is reassigned to the new category. The exact selection rule (and its history-dependence) is detailed in [§7.7](#77-assignment-algorithm). |
| **Offline** | State where `GameState` is `null` (daemon unreachable or non-OK HTTP). Individual buttons render `—` / `OFFLINE`; Dyn Buttons clear to blank. |

---

## 4. System Overview

### 4.1 Runtime and Tooling

| Property | Value | Source |
|----------|-------|--------|
| Plugin UUID | `com.battlestream.streamdeck` | `manifest.json` |
| Manifest `Version` | `1.1.0.0` | `manifest.json` |
| SDK | `@elgato/streamdeck` v2.x | imports |
| `SDKVersion` | `3` | `manifest.json` |
| Minimum Stream Deck app | `7.1` (`Software.MinimumVersion`) | `manifest.json` |
| Node.js | `24` (`Nodejs.Version`, Debug enabled) | `manifest.json` |
| OS minimums | macOS `10.15`, Windows `10` | `manifest.json` |
| Language / bundler | TypeScript, Rollup | repo |
| Code entry | `bin/plugin.js` (`CodePath`) | `manifest.json` |
| Controllers | All Actions are `Keypad`-only | `manifest.json` |

**Implementation note (developers):** All Actions extend `SingletonAction<Record<string, never>>` — there are **no per-instance settings**; configuration is global (see [§4.6](#46-global-settings)).

### 4.2 Data Source and Client

A single `BattlestreamClient` (`src/client.ts`) owns the connection to the daemon. It is constructed with `{host, port, apiKey}` from global settings.

- **Base URL:** `http://{host}:{port}`
- **State fetch (the only payload source):** `GET /v1/game/current`. The full `GameState` JSON **always** comes from this endpoint.
- **Live trigger (SSE):** `GET /v1/events` (EventSource). An SSE message does **not** carry game state; its `onmessage` handler invokes `fetchState()` **fire-and-forget** (it is *not* awaited), which re-fetches `/v1/game/current`. SSE is therefore a change *signal*, not a data channel.
- **Polling fallback:** independently of SSE, a `setInterval` polls `/v1/game/current` every **5 000 ms** (`POLL_INTERVAL`).
- **Connect/reset behavior:** `connect()` clears any pending reconnect/poll timers and resets backoff to `BACKOFF_INITIAL` before (re)opening, then starts the poll interval.
- **Reconnect (verified against `client.ts` `onerror`):** on SSE error the connection closes and a reconnect is scheduled using the **current** backoff. The first reconnect waits **500 ms** (`BACKOFF_INITIAL`); the doubling (`backoff = min(backoff*2, 30000)`) happens *inside* the timer callback *before* re-opening, so each **subsequent** reconnect waits twice the prior delay, capped at **30 000 ms** (`BACKOFF_MAX`). A successful `onopen` resets the delay to 500 ms.
- **Auth:** when `apiKey` is non-empty, requests send `Authorization: Bearer {apiKey}`. When `apiKey` is blank the header is **omitted entirely** (for both the fetch and the SSE handshake).
- **Failure → offline:** a non-OK HTTP response or any thrown error in `fetchState()` reports state `null` (offline).

### 4.3 State Store and Subscription Lifecycle

`StateStore` (`src/state.ts`) is a process-wide singleton (`store`) holding exactly one `GameState | null`.

- **De-duplication:** `setState()` computes a key (`JSON.stringify(state)`, or the literal `null`). If the key matches the previous key, the update is dropped and subscribers are **not** notified. This suppresses identical consecutive refreshes (relevant because both SSE and the 5 s poll cause frequent re-fetches). **A transition from a real game state to `null` (offline), or from `null` back to a game state, always changes the key and is therefore always delivered** — only byte-for-byte identical consecutive states are dropped.
- **Subscription:** actions call `store.subscribe(cb)` and receive an unsubscribe function.
- **Per-Action subscription gating:** each Action class manages its own set of live instances. The store subscription is created **only when the first instance of that Action appears** (`onWillAppear`) and torn down **when the last instance disappears** (`onWillDisappear`). This holds for both the static base class (`BaseStat`) and the Dyn Button class (`DynamicBuffSlotAction`).

### 4.4 GameState Shape

Defined in `src/types.ts`. Field names are snake_case to match the daemon's REST JSON.

```
GameState {
  game_id: string
  phase: "RECRUIT" | "COMBAT" | "GAME_OVER"
  turn: number
  tavern_tier: number
  player: PlayerState
  opponent?: PlayerState        // present only during combat
  board: MinionState[]
  placement: number             // 0 while live, 1–8 at game over
  buff_sources: BuffSource[]    // { category, attack, health }
  ability_counters: AbilityCounter[]  // { category, value, display }
  anomaly_name: string
  is_duos: boolean
  available_tribes?: string[]
}

PlayerState {
  name, hero_card_id,
  health, max_health, damage, armor,
  current_gold, max_gold,
  spell_power, triple_count, tavern_tier,
  win_streak, loss_streak
}
```

> **Note:** the `player.spell_power` field exists in `GameState` but is **not rendered by any button**. The "Spell Power" button shows a different value (the `TAVERN_SPELL` buff source) — see [§6.2](#62-core-stat-buttons). This is recorded in the ambiguities list.

### 4.5 Rendering

`renderButton()` (`src/render.ts`) produces a **144×144 PNG** returned as a base64 `data:` URI. Text is drawn with `textBaseline='middle'`, so the *y* values below are **glyph vertical centers**, not font baselines. Layout, top to bottom:

| Region | Center y (px) | Style | Notes |
|--------|----------|-------|-------|
| **LABEL** | 30 | bold, uppercased, 80% white | Auto-shrinks 17→10 px to fit width (`fitText`). |
| **VALUE** | 82 | bold, white, large | Auto-shrinks 52→14 px to fit. |
| **SUBTITLE** | 122 | normal, 65% white | Only drawn when non-empty; shrinks 14→8 px. |

Background:
- If `iconPath` is set **and** not offline: the icon is drawn full-bleed (144×144) with a **45% black overlay** (`rgba(0,0,0,0.45)`) for text legibility. Loaded images are cached (`imageCache`).
- Otherwise (no icon, icon load failure, or offline): a **two-color diagonal gradient** drawn top-left → bottom-right (`createLinearGradient(0,0,144,144)`), from `gradient[0]` to `gradient[1]`.
- **Offline** forces the gradient path with a fixed desaturated gray gradient (`#2a2a2a` → `#444444`).

### 4.6 Global Settings

`GlobalSettings { host?, port?, apiKey? }` (`src/types.ts`) are configured once via the property inspector `ui/global-settings.html` (the manifest `PropertyInspectorPath`). They apply to **all** buttons. Defaults in the store: `host=127.0.0.1`, `port=8080`, `apiKey=""`.

---

## 5. The Two Kinds of Button

The plugin exposes **two fundamentally different kinds of Action**. The conceptual contrast (what a streamer needs) comes first; the implementation contrast (for developers) follows.

**Concept contrast (everyone):**

| | (A) Individual / Static | (B) Dynamic ("Dyn Button") |
|---|---|---|
| Metric shown | Fixed — always the same stat | Auto-fills with whatever's active |
| Placement | One key = one fixed metric | Place as many as you like; they share a pool |
| Number of these in the action list | Many (one per concept) | Exactly one |
| When the tracker is unreachable | Shows `—` / `OFFLINE` | Goes blank (plain black) |

> **Mental model:** think of individual buttons as **reserved parking** — one car, one spot, always the same car. Dyn Buttons are a **shared lot** the active buffs pull into and out of as the game goes.

**Implementation contrast (developers):**

| | (A) Individual / Static | (B) Dynamic ("Dyn Button") |
|---|---|---|
| Metric shown | Fixed at design time | Auto-assigned at runtime |
| Manifest UUIDs | One per concept (e.g. `…health`, `…bloodgem-buff`) | Single (`…buff-slot`) |
| Icon | Dedicated per Action (`imgs/actions/<id>.png`) | Per-slot icon chosen at render time from the assigned category |
| Implementation | `BaseStat` subclasses (mostly) | `DynamicBuffSlotAction` singleton |

These two kinds are **complementary** and may coexist (see [§9](#9-why-both-kinds-exist-design-intent)). A single category may appear *simultaneously* on its pinned individual button **and** on a Dyn Button — there is no de-duplication between the two kinds.

---

## 6. Individual (Static) Buttons

### 6.1 Why They Are Individual

Each individual button is a **separate manifest Action with its own UUID and its own icon**, and it **always shows one fixed metric**. This is a deliberate design choice:

1. **Determinism & user control.** The user decides exactly which key shows which metric, and that mapping never changes during play. Health is always on the health key.
2. **Always-present metrics.** Core stats (health, gold, tavern tier, turn, phase, …) exist in *every* game on *every* turn. There is no reason to make them compete for a shared slot.
3. **Dedicated iconography.** Each Action ships its own icon (`imgs/actions/<id>.png`), drawn full-bleed behind the value. A stable UUID is what lets Stream Deck associate that icon and lets the user find the button in the action list.
4. **Persistent layouts/profiles.** Because the UUID is stable, a saved profile keeps working across plugin updates.

**Implementation note (developers).** Most individual buttons extend `BaseStat` (`src/actions/base.ts`), implementing only:

```ts
protected abstract label: string;
protected abstract gradient: readonly [string, string];
protected abstract extract(state: GameState): { value: string; subtitle: string };
```

`BaseStat` handles subscription gating, offline rendering, and a **per-instance render cache** (`lastRenderKeys`, keyed by `value|subtitle|isNull`) that skips redundant `setImage` calls. The icon path is derived from the Action's manifest UUID suffix: `imgs/actions/<lastDotSegment>.png`.

**Behavior when value is empty/zero.** Individual buttons render even when their value is zero or empty — they show `—`, `+0/+0`, or a sensible placeholder rather than disappearing. When state is `null`, `BaseStat` renders value `—`, subtitle `OFFLINE`.

### 6.2 Core Stat Buttons

Manifest UUIDs are `com.battlestream.streamdeck.<id>`. All labels and gradients below are the exact literals from each action file.

| Action `Name` | UUID suffix | Label | Gradient | Value / extract logic | Subtitle |
|---|---|---|---|---|---|
| Health | `health` | `HEALTH` | `#7b0000`→`#c0392b` | `player.health − player.damage`; `—` if `max_health==0` | `/ {max_health}` (empty in the `—` case) |
| Armor | `armor` | `ARMOR` | `#3d0000`→`#922b21` | `player.armor` | — |
| Tavern Tier | `tavern-tier` | `TIER` | `#1a3a00`→`#27ae60` | `player.tavern_tier`; `—` if `0` | — |
| Gold | `gold` | `GOLD` | `#5c4a00`→`#d4a017` | `player.current_gold`; `—` if `max_gold==0` | `/ {max_gold}` (empty in the `—` case) |
| Triples | `triples` | `TRIPLES` | `#2d0060`→`#8e44ad` | `player.triple_count` | — |
| Win Streak | `win-streak` | `WIN STR.` | `#003366`→`#2980b9` | `player.win_streak` | — |
| Loss Streak | `loss-streak` | `LOSS STR.` | `#4a2000`→`#e67e22` | `player.loss_streak` | — |
| Placement | `placement` | `PLACE` | `#00474a`→`#16a085` | `#{placement}` when `placement > 0`; **`—` while live** (`placement == 0`) | — |
| Spell Power | `spell-power` | `SPELL PWR` | `#4a004a`→`#a93226` | **`TAVERN_SPELL` buff source** rendered as `+{attack}/+{health}` (`+0/+0` if absent). Does **not** read `player.spell_power`. | — |
| Turn | `turn` | `TURN` | `#1a1a3a`→`#5d6d7e` | `turn`; `—` if `0` | — |
| Phase | `phase` | `PHASE` | `#1a0030`→`#6c3483` | short label for `phase` | — |
| Minion Count | `minion-count` | `MINIONS` | `#003030`→`#1abc9c` | `board.length` | `/ 7` |
| Anomaly | `anomaly` | `ANOMALY` | `#1a1a1a`→`#566573` | `anomaly_name` (or `None`). See precise wrap rule below. | wrap continuation |
| Minions Sold | `minions-sold` | `SOLD` | `#1a0a00`→`#c0392b` | `ability_counters[MINIONS_SOLD].value`, else `0` | — |
| Spells Cast | `spells-cast` | `SPELLS` | `#1a001a`→`#8e44ad` | `ability_counters[SPELLS_CAST].value`, else `0` | — |
| Spellcraft Cast | `spellcraft-cast` | `CRAFT` | `#0d001a`→`#6c3483` | `ability_counters[SPELLCRAFT_CAST].value`, else `0` | — |
| Spellcraft | `spellcraft` | `STACKS` | `#2a0030`→`#9b59b6` | `ability_counters[`**`NAGA_SPELLS`**`].value`, else `0` | — |

> **Anomaly wrap rule (exact, `anomaly.ts`).** Names ≤ 10 chars render on the value line only (no subtitle). For longer names: the **value** line shows characters **0–9** (first 10), and the **subtitle** shows characters **10–19** (up to 10 continuation chars). A trailing `…` is appended to the subtitle **only when `name.length > 20`**. Boundary: a 20-char name shows chars 10–19 with **no** ellipsis; a 21-char name drops the last char and shows `…`.

> **Spell Power — value source and label caution.** The `spell-power` button reads the **`TAVERN_SPELL` buff source** and renders the cumulative `+ATK/+HP` that Tavern Spells (Spellcraft) have granted your **minions** — i.e. a **board-buff total**. It does **not** read `player.spell_power`. In Hearthstone, "Spell Power" / "Spell Damage" is a distinct keyword that boosts spell damage, so the label `SPELL PWR` is a **misnomer** for what is actually a board-buff total and will mislead a BG player. The daemon's own display name for this category is **"Tavern Spells"** (`categories.go:169`), which is accurate; aligning the label with the daemon is **recommended**. This button also **doubles as the individual counterpart of the Dyn-eligible `TAVERN_SPELL` category** (see [§8](#8-the-key-rule-both-forms-always-exist)), despite being grouped here with the core stats. The label concern, and the fact that `player.spell_power` is unrendered, are recorded in the ambiguities list.

> **Note (Spellcraft vs. Spellcraft Cast).** The **Spellcraft** button (`spellcraft`, label `STACKS`) reads the **`NAGA_SPELLS`** ability counter, which is **not** a member of `CATEGORY_META`, so this button has **no Dyn Button counterpart**. It is distinct from **Spellcraft Cast** (`spellcraft-cast`, label `CRAFT`, counter `SPELLCRAFT_CAST`), which *does* have a Dyn counterpart. See [§6.5](#65-the-three-spell-counters) for what each spell counter measures.

### 6.3 Per-Category Buff Buttons

One individual Action per buff category. Each looks up its single category in `buff_sources` and renders `+ATK/+HP` (model: `bloodgem-buff.ts`), or `+0/+0` when absent.

| Action `Name` | UUID suffix | Category looked up |
|---|---|---|
| Bloodgem Buff | `bloodgem-buff` | `BLOODGEM` |
| Elemental Buff | `elemental-buff` | `ELEMENTAL` |
| Tavern-Wide Buff | `tavern-wide-buff` | **sum** of `NOMI_ALL`+`SHOP_BUFF`+`GENERAL` (`TAVERN_WIDE_CATEGORIES`) |
| BG Barrage Buff | `bg-barrage-buff` | `BLOODGEM_BARRAGE` |
| Rightmost Buff | `rightmost-buff` | `RIGHTMOST` |
| Nomi Buff | `nomi-buff` | `NOMI` |
| Undead Buff | `undead-buff` | `UNDEAD` |
| Lightfang Buff | `lightfang-buff` | `LIGHTFANG` |
| Whelp Buff | `whelp-buff` | `WHELP` |
| Beetle Buff | `beetle-buff` | `BEETLE` |
| Volumizer Buff | `volumizer-buff` | `VOLUMIZER` |
| Consumed Buff | `consumed-buff` | `CONSUMED` |

The **Tavern-Wide Buff** button is an aggregate: it sums `attack`/`health` across `NOMI_ALL`, `SHOP_BUFF`, and `GENERAL` (`tavern-wide-buff.ts`, `TavernWideBuffAction`).

> **Game note — the two faces of Nomi.** Nomi buffs are tracked in **two different buckets** depending on which Nomi the player has:
> - **Regular Nomi** ("Nomi, Kitchen Nightmare", `BGS_104`) buffs **Elementals in the tavern** and maps to category **`NOMI`** → the **Nomi Buff** button.
> - **Timewarped Nomi** (in-game card; enchantment `BG34_855pe`, internal code label "KitchenDream") buffs **all minions in the tavern** and maps to category **`NOMI_ALL`**, which is rolled into the **`TAVERN_WIDE`** aggregate → the **Tavern-Wide Buff** button.
>
> So a player running Nomi may see their buffs split across the **Nomi Buff** and **Tavern-Wide Buff** keys depending on which Nomi version is in play. (Verified against `internal/gamestate/categories.go` lines 34–35. "KitchenDream" is an internal code label, not the in-game card name.)

> **Display-name drift (cross-reference note).** Plugin display names (`CATEGORY_META`) can differ from the daemon TUI's `CategoryDisplayName` for the same underlying category. Notable cases:
> - `TAVERN_SPELL`: plugin **`SPELL PWR`** vs. daemon **"Tavern Spells"**.
> - `SPELLS_CAST`: plugin **"Spells Cast"** vs. daemon **"Spells Played"**.
> - `NAGA_SPELLS`: plugin **"STACKS"** vs. daemon **"Naga Stacks"**.
> - `NOMI_ALL`: never surfaced directly by the plugin (folded into **"TVN WIDE"**) vs. daemon **"Nomi Dream"** — so Timewarped-Nomi buffs appear under different headings across the two tools.
>
> A player cross-referencing the TUI against a Stream Deck key should expect these gaps.

### 6.4 Opponent and Summary Buttons

| Action `Name` | UUID suffix | Label | Gradient | Logic |
|---|---|---|---|---|
| Opponent Health | `opponent-health` | `OPP HP` | `#4a0070`→`#8e44ad` | `opponent.health − opponent.damage`; **`—` with empty subtitle** when there is no opponent or `max_health==0`; otherwise value with subtitle `/ {max_health}`. Opponent is present only during combat. |
| Opponent Tavern Tier | `opponent-tavern-tier` | `OPP TIER` | `#1a2a50`→`#2980b9` | opponent tavern tier; `—` when no opponent or tier `0` |
| Available Tribes | `tribes` | `TRIBES` | per source | value = count of `available_tribes`; subtitle = comma-joined 3-letter abbreviations (`TRIBE_ABBR`). When the joined list exceeds **14** chars it is truncated to **13 content characters plus a trailing `…`** (`full.slice(0, SUBTITLE_MAX-1) + '…'`, `SUBTITLE_MAX=14`). `0`/`NONE` when empty. |
| Total Buffs | `total-buffs` | `ALL BUFFS` | `#1a1a2e`→`#8b0000` | sum of `attack`/`health` across **all** `buff_sources`. **The subtitle is always `all buffs`.** The value is `+ATK/+HP` when the sum is non-zero, else `—` (when there are no buff sources or the total is `+0/+0`). |

### 6.5 The Three Spell Counters

BG exposes three distinct spell-related quantities; the plugin surfaces all three as separate buttons. They are **not** interchangeable:

| Button (UUID suffix) | Counter category | What it measures |
|---|---|---|
| Spells Cast (`spells-cast`, label `SPELLS`) | `SPELLS_CAST` | All Tavern spells played this game (daemon: "Spells Played"). |
| Spellcraft Cast (`spellcraft-cast`, label `CRAFT`) | `SPELLCRAFT_CAST` | Spellcraft spells specifically. |
| Spellcraft (`spellcraft`, label `STACKS`) | `NAGA_SPELLS` | The running Naga spell-stack count (tag 3809; daemon: "Naga Stacks") that Naga payoffs scale off of. **Not** in `CATEGORY_META`, hence no Dyn counterpart. |

---

## 7. The Dynamic Button ("Dyn Button")

> **Reader signpost.** *Streamers:* read [§7.2](#72-purpose) (what it's for), [§7.8](#78-rendering-of-dyn-buttons--what-youll-see) (what you'll see), and [§7.9](#79-worked-example-over-a-game) (worked example), and skip the rest. Everything marked **Implementation note** is for developers.

### 7.1 Identity and Naming

The **Dyn Button** (the auto-filling buff key) is **currently labeled "Buff Slot"** in the Stream Deck action list. This document presents its name as **Dyn Button** (the requested rename); until the rename ships, drag on the action still labeled **"Buff Slot."**

| Attribute | Current value (as-built) | This spec presents it as | Notes |
|---|---|---|---|
| Manifest `Name` (display) | `Buff Slot` | **`Dyn Button`** | The user wants the display name changed to **Dyn Button**. This document refers to it as **Dyn Button** throughout. The manifest currently still reads `Buff Slot`. |
| Manifest `UUID` | `com.battlestream.streamdeck.buff-slot` | unchanged | The UUID is the stable internal identifier. |
| Implementing class | `DynamicBuffSlotAction` (`src/actions/buff-slot.ts`) | unchanged | |
| Manifest `Icon` | `imgs/category` | unchanged | This reuses the plugin's **category icon** (shared with `CategoryIcon`), not a dedicated per-action icon — so in the action list the Dyn Button currently looks identical to the plugin's category header. Per-slot icons are chosen at render time from the assigned category (`categoryIconPath` → `imgs/actions/<iconFile>`). |

> **"Dyn Button"** is short for **Dynamic** — an auto-filling buff key.

> **Display name is still under review.** "Dyn" is developer shorthand; a non-technical streamer browsing the Stream Deck action list may not understand that this is the auto-filling buff key. More self-describing candidates (e.g. "Auto Buff", "Dynamic Buff") are under consideration; the final name is an open question (ambiguities item 2). Note also that the manifest currently defines **no `Tooltip`** for this Action — there is no in-app description text to compensate for an ambiguous name. Whether to add a plain tooltip such as *"Auto-fills with whichever buffs are active this game"* is recorded in the ambiguities list.

> **For existing users:** If you already placed "Buff Slot" keys on your deck, they keep working unchanged after this rename — only the name shown in the action list changes from "Buff Slot" to "Dyn Button". You do **not** need to re-add anything.

> Whether to also change the **UUID** (and/or the class/file names) is **out of scope** for this as-built spec and is recorded in the ambiguities list. Changing a UUID breaks existing user layouts and the bundled profiles, so it is not done here. The feature itself is **already implemented and working**; this rename requires no code change beyond the manifest `Name` string.

### 7.2 Purpose

The Dyn Button targets the **long tail of situational categories** — buffs and counters that are **not present in every game**. Their availability is gated by different axes:

- **Tribe-gated:** e.g. Whelps (Dragons) — present only when that tribe is in the lobby.
- **Card/mechanic-gated:** e.g. Bloodgem Barrage; Spellcraft (Nagas); **Beetles** (Beast tokens from Hunter Beetle-payoff cards — present only when you pick up a Beetle payoff, not merely when Beasts are in the lobby) — present only when you buy/play the relevant cards.
- **Anomaly-gated:** certain effects only appear under specific anomalies.

Rather than forcing the user to dedicate one fixed key to *every* possible situational buff, the user places **N Dyn Buttons**. The plugin fills them with whichever categories are currently **active** and **rotates** the assignments as the game evolves. The user reserves a few keys instead of dozens. (Precisely, there are **16 Dyn-eligible categories** — see [Appendix B](#appendix-b-category_meta-reference) — but only a few are ever active at once.)

### 7.3 Eligible Categories

**Implementation note (developers):** the eligible set is `DYNAMIC_CATEGORIES = new Set(Object.keys(CATEGORY_META))` (`src/categories.ts`). Every key of `CATEGORY_META` is eligible; the table below groups them.

| Group | Categories | Render kind |
|---|---|---|
| **Targeted buffs** (`TARGETED`) | `BLOODGEM`, `BLOODGEM_BARRAGE`, `RIGHTMOST` | `+ATK/+HP` |
| **Type buffs** (`TYPE_BUFFS`) | `NOMI`, `ELEMENTAL`, `UNDEAD`, `LIGHTFANG`, `WHELP`, `BEETLE`, `VOLUMIZER`, `CONSUMED`, plus `TAVERN_SPELL` (display `SPELL PWR`) | `+ATK/+HP` |
| **Tavern-wide aggregate** (`TAVERN_WIDE`) | `TAVERN_WIDE` (sums `NOMI_ALL`+`SHOP_BUFF`+`GENERAL`) | `+ATK/+HP` (summed) |
| **Ability counters** (in `TYPE_BUFFS`, `isAbilityCounter`) | `MINIONS_SOLD`, `SPELLS_CAST`, `SPELLCRAFT_CAST` | integer count |

Each `CATEGORY_META` entry carries: `displayName`, `group`, `gradient`, optional `iconFile`, optional `aggregateCategories`, optional `isAbilityCounter`. See [Appendix B](#appendix-b-category_meta-reference) for the full table.

### 7.4 What the Three Buff Groups Mean

The `group` field (`TARGETED` / `TYPE_BUFFS` / `TAVERN_WIDE`) mirrors the daemon's `GroupTargeted` / `GroupTypeBuffs` / `GroupTavernWide` (`internal/gamestate/categories.go`). Grouping is by **buff flavor**, not strictly by whether the player aims the buff. In BG terms:

- **TARGETED** — **Blood-Gem-flavored and position-based** buffs. The one genuinely positional entry is **`RIGHTMOST`** (BG34_854pe), which tracks buffs granted to **your rightmost minion** (a positional payoff). `BLOODGEM` is Blood-Gem stat tracking; `BLOODGEM_BARRAGE` (BG34_689) **adds Blood Gems to refreshed tavern minions on every refresh** — it is grouped here for its Blood-Gem flavor, *not* because the player aims it at a chosen minion.
- **TYPE_BUFFS** — buffs that **scale with a minion type/tribe or a named-minion synergy**: Elementals; **Undead** (tribe-wide Undead buffs, e.g. Nerubian Deathswarmer); Whelps (Dragons); Beetles (Beast tokens from Hunter payoff cards); **Volumizers** (a Mech/Magnetic "your Volumizers" stacking buff); Consumed; Lightfang's per-type buff; and Tavern-Spell minion buffs.
- **TAVERN_WIDE** — buffs applied **broadly across your shop/board**: Timewarped Nomi (`NOMI_ALL`) and generic shop/board buffs (`SHOP_BUFF`, `GENERAL`). Because several broad sources contribute, this group is presented as an **aggregate** (`TAVERN_WIDE` sums them).

### 7.5 Active-Category Definition

`assign()` computes the active set on **every** state update. A category is **active** iff (per `buff-slot.ts`):

| Category kind | Active condition |
|---|---|
| Normal buff category (in `DYNAMIC_CATEGORIES`) | a `buff_sources` entry exists with `attack !== 0` **OR** `health !== 0` |
| Aggregate (has `aggregateCategories`, i.e. `TAVERN_WIDE`) | summed `atk !== 0` **OR** summed `hp !== 0` |
| Ability counter (`isAbilityCounter`) | an `ability_counters` entry exists with `value > 0` |

When `state === null`, the active set is irrelevant: **all slots are cleared** (`slots.clear()`).

### 7.6 Placement and the Fill Region

- A user may place the Dyn Button on **as many keys as desired**. All instances are managed by **one** `DynamicBuffSlotAction` singleton (Stream Deck instantiates the class once and routes all instances' events to it).
- The **fill region** is all current instances sorted **row-major**: top-left fills first, then left-to-right, then down. **In plain terms: even if you scatter Dyn Buttons across the deck, they still fill in reading order — the highest, leftmost Dyn Button gets the first active buff, then the next one to its right, then the first one on the next row down.**

**Fill-order diagram.** Imagine a 3×3 deck with Dyn Buttons (`D`) at `(0,0)`, `(0,2)` and `(1,1)`, and other (non-Dyn) buttons (`·`) elsewhere. The numbers show the order in which active buffs fill them:

```
 col:  0     1     2
row 0 [D1]  [·]   [D2]
row 1 [·]   [D3]  [·]
row 2 [·]   [·]   [·]
```

Fill order is **top row left-to-right, then the next row down** — regardless of where the non-Dyn buttons sit. So the first active buff lands on `(0,0)` → `D1`, the second on `(0,2)` → `D2`, the third on `(1,1)` → `D3`.

- **Implementation note (developers):** each instance is tracked by its `action.id` in three maps (declared in source order):
  - `slots: id → {category, lastUpdated}` — the current binding (absent ⇒ blank). **Map insertion order = the order categories were first bound to slots** (see eviction note in [§7.7](#77-assignment-algorithm)).
  - `coords: id → {row, col}` — grid coordinates captured from the `onWillAppear` payload `coordinates`. **`coords` is set for every live instance** in `onWillAppear` (destructured with `{ row = 0, column = 0 }`), so the `{0,0}` default only applies when Stream Deck omits `coordinates` for an instance. The corresponding fallback inside `sortedIds()` is a defensive no-op under normal flow.
  - `actionMap: id → ActionLike` — the live instances, used for rendering.
  - Row-major sort key: `sortKey = row*1000 + col`, ascending (`sortedIds()`). This governs **free-slot selection only**, not eviction (see [§7.7](#77-assignment-algorithm)).
- **Subscription gating:** the first appearing instance creates the store subscription; the last disappearing instance tears it down. Each `onWillAppear` also immediately runs an assign+render against the current state, so a freshly-placed button populates without waiting for the next update.

### 7.7 Assignment Algorithm

> **Plain version (for streamers):** Active buffs fill empty Dyn Buttons in reading order (top row left-to-right, then the next row). If they're all full and a new buff appears, one occupied button is reused for the new buff. A buff that drops to zero clears its button. Going offline blanks them all. Place more Dyn Buttons so fewer buffs get replaced.

`assign(state)` runs on every state update, **before** rendering (verbatim to `buff-slot.ts`):

1. **Offline short-circuit.** If `state === null`: `slots.clear()`, return. (All Dyn Buttons render blank.)
2. **Compute the active set** per [§7.5](#75-active-category-definition), capturing `now = Date.now()` (wall-clock millis) used for all timestamp bookkeeping this pass.
3. **Clear deactivated slots.** For each existing slot whose category is **no longer active**, delete the slot (that Dyn Button becomes blank).
4. **Refresh retained slots.** For each slot whose category is still active, set `slot.lastUpdated = now`.
5. **Place newly-active categories.** For each active category not currently assigned to any slot, in iteration order of the `active` map:
   - Find the **first FREE instance in row-major order** via `sortedIds()`. If one exists, bind the category there with `lastUpdated = now`. **(This free-slot path is the only place grid/row-major order is consulted.)**
   - Otherwise (**all Dyn Buttons occupied**): scan the `slots` Map for the smallest `lastUpdated` and reassign that slot to the new category with `lastUpdated = now`. This is the **eviction** step.

**Eviction behavior — as built (verified, important).** The eviction scan iterates the **`slots` Map** (`for (const [id, slot] of this.slots)`), not the `actionMap` of registered instances. Step 4 stamps **every retained, still-active slot** with the same `now` *before* step 5 runs; and eviction only executes when no slot is free — meaning **all occupied slots are active and therefore all carry `lastUpdated === now`**. The scan (`if (slot.lastUpdated < lruTime)`, starting at `Infinity`, strict `<`, no update on ties) therefore deterministically selects the **first entry in `slots`-Map iteration order**. That iteration order is **slot-insertion order: the order in which categories were first bound to slots**, which is row-major *at the time of binding* — **not** the registration order of instances, and **not** guaranteed to stay row-major afterward.

Consequently the eviction policy is, as built:

- **NOT recency-ordered** (no slot is "older" than another under contention — they share `now`), and
- **history-dependent on the assignment/eviction sequence.** When a slot is freed (its category went inactive) and later re-filled by a newly-active category, the re-filled binding moves to the **end** of `slots`-Map insertion order — making that slot the **last**, not first, candidate for eviction. So the slot evicted under contention is "the **first-FILLED** slot in current `slots`-Map order," which only coincides with the top-left/row-major key when no slot has been freed-and-refilled during the game.

This is almost certainly **not** the intended policy. Whether eviction should instead use a deterministic, position-stable key (e.g. row-major grid position) or true recency (recency tracked *before* the step-4 refresh) is an open question recorded in the ambiguities list.

**Other consequences:**
- A category that goes inactive frees its slot immediately; that slot is then available (blank) for the next newly-active category.
- The number of categories shown at once is bounded by the number of placed Dyn Buttons. **More Dyn Buttons ⇒ more situational categories visible simultaneously; fewer ⇒ contention triggers eviction and only the survivors remain visible.**

### 7.8 Rendering of Dyn Buttons — What You'll See

> **What you'll see — at a glance.** A Dyn key that is simply unused shows **nothing** (no text). A key that can't reach the tracker shows a **dash `—`**. The unambiguous, eye-level rule: **a dash means a connection problem; nothing means an unused slot.** A blank Dyn Button is never an "error" tile.

| What you see on the key | What it means | What to do |
|---|---|---|
| **Plain black, no text on it** (a Dyn Button) | No extra buff is assigned to this slot yet | Nothing — this is normal |
| **Colored, with a buff name on top and numbers below** | A buff is active and assigned to this Dyn Button | Nothing |
| **Shows a dash `—`** (greyed-out *individual* button, or any key) | The plugin can't reach your tracker | Check the daemon is running and your connection settings |

**Implementation note (developers).** `renderAll()` renders every instance each update; `renderOne()` decides per instance:

| Instance state | Render |
|---|---|
| **No slot** (unassigned/blank) | Solid black (`gradient ['#000000','#000000']`), empty label/value/subtitle, `offline:false`. A **blank placeholder — NOT the `OFFLINE` tile.** |
| **Assigned, ability counter** | label = `displayName`; value = `ability_counters[cat].value` as integer (`0` if missing); category gradient + icon |
| **Assigned, aggregate** (`TAVERN_WIDE`) | value = `+{atk}/+{hp}` summed over `aggregateCategories` |
| **Assigned, normal buff** | value = `+{attack}/+{health}` from the matching `buff_sources` entry (`+0/+0` if missing) |

Assigned slots use the category's `displayName` as the label, its `gradient`, and its icon via `categoryIconPath(cat)` (`imgs/actions/<iconFile>`). Because `assign()` clears all slots when state is null, `renderOne()` can assume non-null state whenever a slot exists.

### 7.9 Worked Example Over a Game

Assume the user placed **2** Dyn Buttons, scattered on different rows: the **left** one at grid `(row0,col1)` and the **right** one at `(row1,col0)`. Row-major order is `row*1000+col`, so `(0,1)=1` fills before `(1,0)=1000` — the **left/top** key fills first even though the other is further left, because it is on a higher row. Below, each row shows the two physical keys as `[ left/top ] [ right/lower ]`.

| Game moment | Active categories | Physical keys (left-top / right-lower) | What happened |
|---|---|---|---|
| Early recruit, nothing bought | (none) | `[ (blank) ] [ (blank) ]` | No active categories. |
| Buy a bloodgem minion | `BLOODGEM` | `[ Bloodgems +4/+6 ] [ (blank) ]` | First free key in reading order is the left/top one (higher row). |
| Beetles come online | `BLOODGEM`, `BEETLE` | `[ Bloodgems +4/+6 ] [ Beetles +2/+2 ]` | Next free key is the right/lower one. |
| Whelps come online — **both keys full** | `BLOODGEM`, `BEETLE`, `WHELP` | `[ Whelps +3/+3 ] [ Beetles +2/+2 ]` | Both Dyn Buttons are full, so one is reused. Here the Bloodgems key gives way to Whelps. With only 2 Dyn Buttons you can't see all three — **add a 3rd Dyn Button** and Bloodgems, Beetles, and Whelps would all show together. |
| Bloodgem/Whelp buffs grow but stay non-zero | all stay active | values refresh (e.g. `[ Whelps +5/+5 ] [ Beetles +4/+4 ]`) | Still-active keys just refresh their numbers; they are not reused. |
| Beetles drop to `+0/+0` | Beetles inactive | `[ Whelps +5/+5 ] [ (blank) ]` | The Beetles key goes blank because that buff is no longer active. |
| Daemon stops / game ends to offline (`null`) | — | `[ (blank) ] [ (blank) ]` | All Dyn Buttons clear. |

> **Streamer caveat — don't count on *which* key gets reused.** When all your Dyn Buttons are full and a new buff appears, the plugin reuses one of them, but **which one is not guaranteed to follow position** — it depends on the order buffs came and went during the game, so it can look arbitrary. Don't rely on a specific key being the one replaced. If you don't want any buff dropped, just **add more Dyn Buttons.**

> **Implementation note (developers).** In the "both keys full" row, the reused (evicted) key is the one whose slot is **first in `slots`-Map insertion order** — i.e. the category that was bound to a slot first. Here Bloodgems was assigned before Beetles, so the Bloodgems slot is first in the `slots` Map; under full contention every occupied slot shares `lastUpdated === now` (step 4 of [§7.7](#77-assignment-algorithm)), so the eviction scan picks that first slot. **This outcome is path-dependent:** had Bloodgems earlier gone inactive and been re-assigned later, its slot would have moved to the end of `slots` insertion order and Beetles would be evicted instead — even with the identical physical layout. See the history-dependence note in [§7.7](#77-assignment-algorithm); whether this is the intended policy is an open question (ambiguities item 4). The user-facing takeaway is unchanged: with only 2 Dyn Buttons a third active buff displaces one, so place more to keep more visible.

---

## 8. The Key Rule: Both Forms Always Exist

> **KEY RULE — verified.** Every category that can appear in a Dyn Button **also exists as its own individual standalone Action** in the plugin's action list. Both forms always exist; a user can always pin any specific buff/counter to a fixed key *and/or* let it flow through the Dyn Button pool.

Mapping of each `DYNAMIC_CATEGORIES` member to its dedicated individual Action:

| Dynamic category | Individual Action UUID suffix |
|---|---|
| `BLOODGEM` | `bloodgem-buff` |
| `BLOODGEM_BARRAGE` | `bg-barrage-buff` |
| `RIGHTMOST` | `rightmost-buff` |
| `NOMI` | `nomi-buff` |
| `ELEMENTAL` | `elemental-buff` |
| `UNDEAD` | `undead-buff` |
| `LIGHTFANG` | `lightfang-buff` |
| `WHELP` | `whelp-buff` |
| `BEETLE` | `beetle-buff` |
| `VOLUMIZER` | `volumizer-buff` |
| `CONSUMED` | `consumed-buff` |
| `TAVERN_WIDE` | `tavern-wide-buff` |
| `TAVERN_SPELL` (`SPELL PWR`) | `spell-power` |
| `MINIONS_SOLD` | `minions-sold` |
| `SPELLS_CAST` | `spells-cast` |
| `SPELLCRAFT_CAST` | `spellcraft-cast` |

The rule is **one-directional and complete**: *every Dyn-eligible category has a standalone button.* The converse does **not** hold — some individual buttons (the core stats, the opponent/summary buttons, and `spellcraft`/`NAGA_SPELLS`) have **no** Dyn counterpart, because they are not in `CATEGORY_META`. This asymmetry is expected and recorded in the ambiguities list for confirmation.

> **No de-duplication across kinds.** A category may render on its pinned individual button **and** simultaneously occupy a Dyn Button. The two button kinds do not coordinate.

---

## 9. Why Both Kinds Exist (Design Intent)

- **Individual buttons** are deterministic, user-controlled, fixed in position, and always show the same metric. They are ideal for the handful of stats you *always* want (health, gold, tavern tier) and for any specific buff a player wants permanently pinned.
- **Dyn Buttons** are a pool of shared, auto-populated keys for the **situational long tail** that varies game-to-game. The user reserves a few keys instead of dedicating one to each of the 16 Dyn-eligible categories.
- **They are complementary and coexist.** You can pin Bloodgems to a fixed key *and* still have it appear in the Dyn pool.
- **Scaling knob:** the number of placed Dyn Buttons is the user's tradeoff dial. More Dyn Buttons ⇒ more situational categories visible at once; fewer ⇒ contention causes eviction and only the survivors remain visible.

### 9.1 Should I Pin It or Use a Dyn Button?

| Your situation | Recommendation |
|---|---|
| A stat you want **every game** (health, gold, tavern tier, turn, placement) | **Pin it** — individual button |
| A **specific buff you always care about** (e.g. Bloodgems, Elementals) | **Pin it** — its individual buff button |
| Buffs that **only show up some games** (Beetles, Whelps, Spellcrafts, Barrage…) | **Use Dyn Buttons** |
| You have **only a few free keys** | **Use Dyn Buttons** |

**Headline tradeoff:** *Pinning* = always there, but uses one key per buff. *Dyn Buttons* = auto-fill themselves, but only show as many buffs at once as the number of Dyn Buttons you placed.

**Rule of thumb:** In a typical game only **2–4 buff types are active at once** (your main tribe/synergy plus a couple of incidental ones), which is why **2–4 Dyn Buttons** usually shows everything without dropping anything. Watch a few of your own runs — if you frequently see a buff get replaced and wish it had stayed, add one more Dyn Button.

---

## 10. Bundled Profiles

Per-device profiles ship in `streamdeck-plugin/profiles/` and are declared in the manifest `Profiles` block:

| Profile `Name` (manifest) | `DeviceType` | Typical grid |
|---|---|---|
| Battle Stream Standard | `0` | 5×3 |
| Battle Stream XL | `2` | 8×4 |
| Battle Stream Mini | `1` | 3×2 |
| Battle Stream Plus | `7` | 4×3 (encoder device) |

All four are `ReadOnly:false`, `DontAutoSwitchWhenInstalled:false`. Regeneration and install are documented in `profiles/README.md` (`node scripts/gen-profiles.mjs`; OpenDeck install via `make install-plugin`).

> **Naming divergence (follow-up):** on-disk profile bundles are named **`Battlestream <Variant>.sdProfile`** (one word, "Battlestream") while the manifest `Profile` `Name`s use **`Battle Stream <Variant>`** (two words). This spelling mismatch between the directory/bundle name and the manifest `Name` is noted in the ambiguities list for confirmation that Stream Deck resolves the profiles correctly.

> Prior design docs (`docs/superpowers/specs/2026-05-05-…` and `2026-05-08-…`) are partly **stale**: the original "Auto-Layout" feature and the `buff-atk`/`buff-hp` actions were **removed/deleted**. This specification reflects the **current** manifest only.

---

## 11. Test Coverage

Unit tests live in `streamdeck-plugin/src/__tests__/`:

| Test file | Covers |
|---|---|
| `actions/buff-slot.test.ts` | Dyn Button assignment, row-major fill, eviction, clearing on inactive/offline |
| `actions/static-buffs.test.ts` | Per-category individual buff buttons |
| `actions/total-buffs.test.ts` | `ALL BUFFS` summation |
| `actions/tribes.test.ts` | Tribe abbreviation/truncation |
| `actions/opponent-tiles.test.ts` | Opponent health / tavern tier |
| `actions/base.test.ts` | `BaseStat` lifecycle, offline rendering, render cache |

---

## Appendix A: Complete Manifest Action List

UUIDs are prefixed `com.battlestream.streamdeck.`. Order matches `manifest.json`.

| # | UUID suffix | `Name` | Kind |
|---|---|---|---|
| 1 | `health` | Health | Static core |
| 2 | `armor` | Armor | Static core |
| 3 | `tavern-tier` | Tavern Tier | Static core |
| 4 | `gold` | Gold | Static core |
| 5 | `triples` | Triples | Static core |
| 6 | `win-streak` | Win Streak | Static core |
| 7 | `loss-streak` | Loss Streak | Static core |
| 8 | `placement` | Placement | Static core (value `#N`; `—` while live) |
| 9 | `spell-power` | Spell Power | Static core (renders Dyn **buff** `TAVERN_SPELL`) |
| 10 | `turn` | Turn | Static core |
| 11 | `phase` | Phase | Static core |
| 12 | `minion-count` | Minion Count | Static core |
| 13 | `anomaly` | Anomaly | Static core |
| 14 | `bloodgem-buff` | Bloodgem Buff | Static buff |
| 15 | `elemental-buff` | Elemental Buff | Static buff |
| 16 | `spellcraft` | Spellcraft | Static (`NAGA_SPELLS`; no Dyn counterpart) |
| 17 | `minions-sold` | Minions Sold | Static counter |
| 18 | `spells-cast` | Spells Cast | Static counter |
| 19 | `spellcraft-cast` | Spellcraft Cast | Static counter |
| 20 | `tavern-wide-buff` | Tavern-Wide Buff | Static buff (aggregate) |
| 21 | `bg-barrage-buff` | BG Barrage Buff | Static buff |
| 22 | `rightmost-buff` | Rightmost Buff | Static buff |
| 23 | `nomi-buff` | Nomi Buff | Static buff |
| 24 | `undead-buff` | Undead Buff | Static buff |
| 25 | `lightfang-buff` | Lightfang Buff | Static buff |
| 26 | `whelp-buff` | Whelp Buff | Static buff |
| 27 | `beetle-buff` | Beetle Buff | Static buff |
| 28 | `volumizer-buff` | Volumizer Buff | Static buff |
| 29 | `consumed-buff` | Consumed Buff | Static buff |
| 30 | `buff-slot` | **Buff Slot** → present as **Dyn Button** | **Dynamic** |
| 31 | `opponent-health` | Opponent Health | Static opponent |
| 32 | `opponent-tavern-tier` | Opponent Tavern Tier | Static opponent |
| 33 | `tribes` | Available Tribes | Static summary |
| 34 | `total-buffs` | Total Buffs | Static summary |

## Appendix B: CATEGORY_META Reference

From `src/categories.ts`. `DYNAMIC_CATEGORIES` = all keys below (16 categories). Icons resolve to `imgs/actions/<iconFile>`.

| Category | `displayName` | `group` | `gradient` | `iconFile` | aggregate | counter |
|---|---|---|---|---|---|---|
| `BLOODGEM` | Bloodgems | TARGETED | `#3a1a00`→`#e67e22` | bloodgem-buff.png | — | — |
| `BLOODGEM_BARRAGE` | BG Barrage | TARGETED | `#1a1000`→`#7a5000` | bg-barrage-buff.png | — | — |
| `RIGHTMOST` | Rightmost | TARGETED | `#1a1000`→`#7a5000` | rightmost-buff.png | — | — |
| `NOMI` | Nomi | TYPE_BUFFS | `#120a20`→`#4a3070` | nomi-buff.png | — | — |
| `ELEMENTAL` | Elementals | TYPE_BUFFS | `#3a2a00`→`#f39c12` | elemental-buff.png | — | — |
| `UNDEAD` | Undead | TYPE_BUFFS | `#120a20`→`#4a3070` | undead-buff.png | — | — |
| `LIGHTFANG` | Lightfang | TYPE_BUFFS | `#120a20`→`#4a3070` | lightfang-buff.png | — | — |
| `WHELP` | Whelps | TYPE_BUFFS | `#120a20`→`#4a3070` | whelp-buff.png | — | — |
| `BEETLE` | Beetles | TYPE_BUFFS | `#120a20`→`#4a3070` | beetle-buff.png | — | — |
| `VOLUMIZER` | Volumizer | TYPE_BUFFS | `#120a20`→`#4a3070` | volumizer-buff.png | — | — |
| `CONSUMED` | Consumed | TYPE_BUFFS | `#120a20`→`#4a3070` | consumed-buff.png | — | — |
| `TAVERN_SPELL` | SPELL PWR | TYPE_BUFFS | `#4a004a`→`#a93226` | spell-power.png | — | — |
| `TAVERN_WIDE` | TVN WIDE | TAVERN_WIDE | `#001a26`→`#1a6b8a` | tavern-wide-buff.png | `NOMI_ALL`,`SHOP_BUFF`,`GENERAL` | — |
| `MINIONS_SOLD` | Sold | TYPE_BUFFS | `#1a0a00`→`#c0392b` | minions-sold.png | — | ✓ |
| `SPELLS_CAST` | Spells Cast | TYPE_BUFFS | `#1a001a`→`#8e44ad` | spells-cast.png | — | ✓ |
| `SPELLCRAFT_CAST` | Spellcrafts | TYPE_BUFFS | `#0d001a`→`#6c3483` | spellcraft-cast.png | — | ✓ |

---

## Open Questions

Items requiring a future human decision are tracked in the companion **[Ambiguities & Open Questions](2026-05-31-streamdeck-plugin-spec-ambiguities.md)** document, not invented here. They include the Dyn Button rename scope, the eviction-policy behavior question raised in [§7.7](#77-assignment-algorithm), the `SPELL PWR` label misnomer, and plugin-vs-daemon display-name drift.

---

*End of specification.*
