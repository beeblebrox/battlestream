# Stream Deck Plugin Spec — Open Questions & Ambiguities

> Companion document to the Battlestream Stream Deck Plugin specification. This file enumerates decisions that were **deliberately left unresolved** in the spec because they require a human product/UX/engineering judgment call rather than a documentation choice. Each item states the question, why it matters / what depends on it, and a recommended default where one exists.
>
> Scope note: nothing here is a bug report. Several items describe behavior that is *implemented and working* but whose *intended design* is undecided. Where the code and the spec currently disagree, that is called out explicitly.

---

## Theme 1 — Naming & terminology

### 1.1 Rename "Buff Slot" display name to "Dyn Button" — change the manifest UUID too, or not?

**Question.** The user wants the dynamic action's display name changed from "Buff Slot" to "Dyn Button". Should the manifest Action UUID (`com.battlestream.streamdeck.buff-slot`) and internal identifiers change to match, or should only the human-readable `Name` field change while the UUID stays `buff-slot`?

**Why it matters / what depends on it.** Stream Deck profiles and user key layouts reference actions **by UUID**, not by display name. Changing the UUID `com.battlestream.streamdeck.buff-slot` would orphan every existing placement of this action in:
- the four bundled profiles (Standard 5x3, XL 8x4, Mini 3x2, Plus 4x3) under `streamdeck-plugin/profiles/`, and
- any layouts users have already built by hand.

Keys placed with the old UUID would silently disappear or fail to load after an update. A display-name-only change is purely cosmetic and **safe**.

**Recommended default.** **Change only the `Name` field to "Dyn Button"; keep the UUID `com.battlestream.streamdeck.buff-slot` unchanged.** Treat the UUID as a permanent, opaque identifier. If a UUID change is ever deemed necessary, it must be accompanied by a profile-migration plan and a major-version bump, and the bundled profiles must be regenerated (`node scripts/gen-profiles.mjs`).

### 1.2 Internal-name inconsistency across manifest, class name, and docs

**Question.** The concept currently carries **three** different names. Which (if any) should be reconciled, and how far should the rename reach?

| Layer | Current identifier |
|---|---|
| Manifest `Name` (display) | "Buff Slot" → requested "Dyn Button" |
| Manifest Action `UUID` | `com.battlestream.streamdeck.buff-slot` |
| Source file | `src/actions/buff-slot.ts` |
| Class | `DynamicBuffSlotAction` |
| Prior design docs | "Buff Slot", "dynamic Buff Slot" |

**Why it matters / what depends on it.** Future maintainers grepping for "Dyn Button" will find nothing; grepping for "buff slot" finds the code but not the user-facing label. The mismatch raises onboarding cost and invites someone to "fix" the UUID (see 1.1) by mistake. Conversely, an aggressive rename of the class/file/UUID is exactly the change that risks breaking layouts.

**Recommended default.** Decouple the two axes deliberately and document the decision in-spec:
- **User-facing label:** "Dyn Button" (manifest `Name` only).
- **Internal identifiers (UUID, filename, class `DynamicBuffSlotAction`):** leave as-is for stability; the "buff-slot" lineage is harmless internally.
- Add a one-line note in the spec and in `buff-slot.ts` mapping the three names so the inconsistency is *documented* rather than *accidental*. If a class/file rename is wanted for cleanliness, do it in a separate refactor PR that explicitly does **not** touch the UUID.

### 1.3 Is "Dyn Button" the right streamer-facing name at all?

**Question.** Should the streamer-facing name be "Dyn Button", or a fully self-describing label such as "Auto Buff Key", "Smart Buff", or "Auto Buff Slot"?

**Why it matters / what depends on it.** The spec itself acknowledges that non-technical streamers will not parse "Dyn" (abbreviation of "dynamic"). Every streamer-facing section — Quick Start and the §7 worked example — is written *around this term*. The manifest currently has **no Tooltip** to compensate. If the final name changes, all user-facing prose and headings must be reworded. This decision should therefore be **locked before the user-facing copy is finalized**, otherwise the documentation will be rewritten twice.

**Recommended default.** Prefer a self-describing label (e.g. **"Auto Buff Key"**) and add a manifest **Tooltip** describing the auto-fill/rotation behavior regardless of the chosen name. If "Dyn Button" is kept for brevity, the Tooltip becomes mandatory, not optional. Treat this as a blocking prerequisite for finalizing Quick Start / §7 copy.

---

## Theme 2 — Dynamic-button assignment & eviction behavior

> These are **behavior-design** decisions, not doc fixes. The spec presently documents an *intended* mechanism that does **not** match the shipped code; resolving these determines both the code and the prose.

### 2.1 Eviction policy: deterministic by grid position, true recency, or current history-dependent order?

**Question.** When all Dyn Button slots are full and a new category activates, which occupied slot should be evicted? Options:
1. **True LRU** — evict the genuinely least-recently-*updated* category.
2. **Deterministic by grid position** — e.g. always evict the bottom-right (last in row-major order).
3. **Current behavior** — accept the history-dependent `slots`-Map insertion order as-is.

**Why it matters / what depends on it.** The spec documents an LRU ("evict the Least-Recently-Updated slot") and a "first-registered" framing, but the code does **not** behave as a true LRU. In `buff-slot.ts` (~lines 88–90) every retained, still-active slot is stamped with the **same** `now` timestamp before the eviction scan. That collapses the LRU tiebreak into "first entry in the `slots` Map", whose order depends on prior assignment/eviction **history**. Consequence: **two identical deck layouts can evict different categories** depending on what activated earlier in the game. The behavior is non-reproducible and currently mis-described in the spec.

**Recommended default.** **Adopt deterministic position-based eviction (option 2): evict the last slot in row-major order (bottom-right).** It is teachable, reproducible across identical layouts, and trivially documentable ("the bottom-right Dyn Button is the one that gets replaced"). If true recency is preferred instead, fix the timestamp-stamping so LRU is actually meaningful. Either way, the spec must be rewritten to match the chosen, implemented behavior — the current "LRU/first-registered" prose is inaccurate.

### 2.2 Active-category placement order under contention

**Question.** When several categories become active in the *same* state update and free slots are limited, what priority decides which categories win the available slots? Current code iterates `buff_sources` first, then the `TAVERN_WIDE` aggregate, then `ability_counters` (`buff-slot.ts` ~lines 66–80). Is that an intentional priority or an accident of loop order?

**Why it matters / what depends on it.** This iteration order is the de-facto tiebreak for which buffs surface first when there aren't enough Dyn Buttons. Today it is an **implementation accident** (the order the loops happen to run). If a streamer cares that, say, Blood Gem buffs appear before a "minions sold" counter, that preference has no defined home and could change if the loops are refactored.

**Recommended default.** Define an **explicit priority order** in the spec and pin it in code — recommended: targeted/type buffs (`buff_sources`) → `TAVERN_WIDE` aggregate → ability counters, matching today's behavior but now *by design*. Alternatively, inherit the daemon's `CategoryGroup` ordering for consistency with the rest of the product. Pick one and document it as a stable contract.

### 2.3 Should the eviction rule be exposed to users as "unpredictable" or as a teachable guarantee?

**Question.** Independent of the internal mechanism (2.1), what does the **documentation promise the user**? Either "don't rely on which Dyn Button gets replaced" (hedge), or a concrete, teachable rule ("the bottom-right one is replaced").

**Why it matters / what depends on it.** This determines whether the §7 worked example can promise a **predictable visual outcome** or must hedge. A position-based, teachable rule is dramatically more usable than "unpredictable". This question is effectively the user-facing twin of 2.1 and should resolve **together with it** — the doc cannot promise determinism the code doesn't deliver.

**Recommended default.** Commit to a deterministic, teachable rule (bottom-right eviction, per 2.1) so the worked example can show a concrete outcome. Only fall back to a hedge if the team explicitly declines to make eviction deterministic.

---

## Theme 3 — Category taxonomy (daemon-owned)

### 3.1 Should `BLOODGEM_BARRAGE` stay under the `TARGETED` group?

**Question.** `BLOODGEM_BARRAGE` is grouped under `TARGETED` because the daemon's `CategoryGroup` (`categories.go` ~lines 205–207) places it in `GroupTargeted`, and the plugin inherits that grouping. But mechanically it is a **tavern-wide-on-refresh** Blood Gem effect, not a player-aimed target. Should the grouping change, or should the label be explained?

**Why it matters / what depends on it.** A Battlegrounds player reading "Targeted" reasonably expects a minion **they** select. Grouping a refresh-triggered, board-wide Blood Gem buff under "Targeted" makes the taxonomy read as inaccurate. This is a **daemon-side grouping decision**, not a writer fix — the plugin only mirrors `categories.go`.

**Recommended default.** Keep the daemon grouping as-is for now, but have the spec **explicitly clarify** that the "Targeted" group is organized by **buff flavor (Blood Gems)**, not by user-aimed targeting. If the product wants the taxonomy to read literally, that change belongs in the daemon's `CategoryGroup` and would propagate to the plugin automatically.

---

## Theme 4 — Documentation hygiene follow-ups (flagged, not blocking)

### 4.1 Stale prior-design references

**Question.** The spec inherits context from two prior design docs that are partly stale — `2026-05-05-streamdeck-plugin-design.md` (mentions removed **Auto-Layout** and deleted `buff-atk`/`buff-hp` actions) and `2026-05-08-buff-grouping-design.md`. How should stale sections be handled?

**Why it matters / what depends on it.** Readers cross-referencing the originals may resurrect removed features (Auto-Layout, `buff-atk`/`buff-hp`). The current spec must reflect the **current manifest** only.

**Recommended default.** Mark the stale sections of the prior docs as superseded (header banner) and ensure the new spec links forward, not back, for anything Auto-Layout / `buff-atk` / `buff-hp` related. No design decision required — purely an editorial cleanup, tracked here so it is not forgotten.

---

## Summary of decisions needed from a human

| # | Decision | Owner | Recommended default |
|---|---|---|---|
| 1.1 | Change Dyn Button UUID, or display name only? | Product + Eng | Display name only; keep UUID |
| 1.2 | Reconcile 3-way name inconsistency how far? | Eng | Rename label only; document internal lineage |
| 1.3 | Final streamer-facing name + Tooltip? | UX | "Auto Buff Key" + mandatory Tooltip (blocks copy) |
| 2.1 | Eviction policy: position / true LRU / status quo? | Eng + UX | Deterministic bottom-right eviction |
| 2.2 | Contention placement priority a defined rule? | Eng | Explicit: buff_sources → TAVERN_WIDE → counters |
| 2.3 | Promise users determinism or hedge? | UX | Promise teachable bottom-right rule (ties to 2.1) |
| 3.1 | `BLOODGEM_BARRAGE` group placement/label? | Daemon owner | Keep group; clarify "by buff flavor" in prose |
| 4.1 | Handle stale prior-design sections? | Tech writer | Banner as superseded; forward-link only |
