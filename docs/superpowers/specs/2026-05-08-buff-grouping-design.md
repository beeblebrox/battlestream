# Buff Grouping Redesign

**Date:** 2026-05-08  
**Status:** Approved

## Problem

The BUFF SOURCES panel in the TUI and the corresponding Stream Deck buttons show buff categories as a flat list sorted by `+ATK/+HP` magnitude. The categories are named after their source card (Nomi, Bloodgems, etc.) but carry no signal about *how* a buff applies — to the whole tavern, to one targeted minion, or permanently to a specific minion type. This makes the display hard to read at a glance.

## Design

### Three Buff Groups

All buff source categories are reorganised into three semantic groups:

| Group | Display Name | What belongs here |
|---|---|---|
| `GroupTavernWide` | TAVERN-WIDE | Buffs that apply to all minions regardless of type: NomiAll (Timewarped Nomi), TavernSpell, ShopBuff, General |
| `GroupTargeted` | TARGETED | Buffs that go to one specific or random minion: Bloodgem, BloodgemBarrage, Rightmost |
| `GroupTypeBuffs` | TYPE BUFFS | Permanent buffs earned by playing to a tribe or repeating an action: Nomi, Elemental, Undead, Lightfang, Whelp, Beetle, Volumizer, Consumed |

Ability counters (NagaSpells/Spellcraft, FreeRefresh, GoldNextTurn) remain in their own ABILITIES section and are not affected.

### Category → Group Mapping

Added to `internal/gamestate/categories.go`:

```go
const (
    GroupTavernWide = "TAVERN_WIDE"
    GroupTargeted   = "TARGETED"
    GroupTypeBuffs  = "TYPE_BUFFS"
)

var CategoryGroup = map[string]string{
    CatNomiAll:         GroupTavernWide,
    CatTavernSpell:     GroupTavernWide,
    CatShopBuff:        GroupTavernWide,
    CatGeneral:         GroupTavernWide,

    CatBloodgem:        GroupTargeted,
    CatBloodgemBarrage: GroupTargeted,
    CatRightmost:       GroupTargeted,

    CatNomi:            GroupTypeBuffs,
    CatElemental:       GroupTypeBuffs,
    CatUndead:          GroupTypeBuffs,
    CatLightfang:       GroupTypeBuffs,
    CatWhelp:           GroupTypeBuffs,
    CatBeetle:          GroupTypeBuffs,
    CatVolumizer:       GroupTypeBuffs,
    CatConsumed:        GroupTypeBuffs,
}
```

No changes to `BuffSource` struct, state machine, proto, or API. Grouping is a pure display concern.

### TUI Panel (`internal/tui/tui.go` — `modsItems()`)

The panel renders three sections in order. Sections with no active (non-zero) categories are omitted.

```
TAVERN-WIDE
  +18/+14

TARGETED
  Bloodgems     +4/+0
  Rightmost     +2/+2

TYPE BUFFS
  Undead        +6/+6
  Nomi          +4/+4
  Elementals    +2/+2
  Lightfang     +3/+3

ABILITIES
  Bonus Gold    +2
  Refreshes     3 free
```

**TAVERN-WIDE** renders as a single summed `+ATK/+HP` value (all contributing categories summed). Individual contributors are not listed.

**TARGETED** and **TYPE BUFFS** render each active category as its own line with `+ATK/+HP`.

Section headers use the existing group color: teal for TAVERN-WIDE, gold for TARGETED, purple for TYPE BUFFS.

---

## Stream Deck Plugin Changes

### Actions Removed

| UUID | Name | Reason |
|---|---|---|
| `com.battlestream.streamdeck.buff-atk` | Buff ATK | Replaced by Tavern-Wide Buff |
| `com.battlestream.streamdeck.buff-hp` | Buff HP | Replaced by Tavern-Wide Buff |
| `com.battlestream.streamdeck.tavern-spell-buff` | Tavern Spell Buff | Rolled into Tavern-Wide Buff |
| `com.battlestream.streamdeck.auto-layout` | Auto-Layout | Removed per user request |

### New Static Buttons

All categories that appear in the dynamic slot also have a permanent static button a user can place if they want it always visible.

#### Tavern-Wide Buff (new — static)
- UUID: `com.battlestream.streamdeck.tavern-wide-buff`
- Shows combined `+ATK/+HP` summing all TAVERN-WIDE categories
- Icon: repurpose `buff-atk.png` · teal gradient `['#001a26', '#1a6b8a']`

#### Targeted — Static Buttons

| Action file | UUID suffix | Category | Icon |
|---|---|---|---|
| `bloodgem-buff.ts` | `bloodgem-buff` | BLOODGEM | `bloodgem-buff.png` ✓ (kept, no change) |
| `bg-barrage-buff.ts` | `bg-barrage-buff` | BLOODGEM_BARRAGE | gold gradient (no icon) |
| `rightmost-buff.ts` | `rightmost-buff` | RIGHTMOST | gold gradient (no icon) |

#### Type Buffs — Static Buttons

| Action file | UUID suffix | Category | Icon |
|---|---|---|---|
| `elemental-buff.ts` | `elemental-buff` | ELEMENTAL | `elemental-buff.png` ✓ (kept, no change) |
| `nomi-buff.ts` | `nomi-buff` | NOMI | purple gradient (no icon) |
| `undead-buff.ts` | `undead-buff` | UNDEAD | purple gradient (no icon) |
| `lightfang-buff.ts` | `lightfang-buff` | LIGHTFANG | purple gradient (no icon) |
| `whelp-buff.ts` | `whelp-buff` | WHELP | purple gradient (no icon) |
| `beetle-buff.ts` | `beetle-buff` | BEETLE | purple gradient (no icon) |
| `volumizer-buff.ts` | `volumizer-buff` | VOLUMIZER | purple gradient (no icon) |
| `consumed-buff.ts` | `consumed-buff` | CONSUMED | purple gradient (no icon) |

All new static buttons extend `BaseStat` and implement `extract()` by looking up their category in `s.buff_sources`.

### Shared Category Metadata (`src/categories.ts`)

A new shared module provides the TypeScript-side category metadata consumed by both static buttons and the dynamic slot:

```typescript
export interface CategoryMeta {
  displayName: string;
  group: 'TAVERN_WIDE' | 'TARGETED' | 'TYPE_BUFFS';
  gradient: readonly [string, string];
  iconFile?: string; // filename under imgs/actions/, if one exists
}

export const CATEGORY_META: Record<string, CategoryMeta> = {
  BLOODGEM:        { displayName: 'Bloodgems',    group: 'TARGETED',    gradient: [...], iconFile: 'bloodgem-buff.png' },
  BLOODGEM_BARRAGE:{ displayName: 'BG Barrage',   group: 'TARGETED',    gradient: ['#1a1000','#7a5000'] },
  RIGHTMOST:       { displayName: 'Rightmost',    group: 'TARGETED',    gradient: ['#1a1000','#7a5000'] },
  ELEMENTAL:       { displayName: 'Elementals',   group: 'TYPE_BUFFS',  gradient: [...], iconFile: 'elemental-buff.png' },
  NOMI:            { displayName: 'Nomi',         group: 'TYPE_BUFFS',  gradient: ['#120a20','#4a3070'] },
  UNDEAD:          { displayName: 'Undead',       group: 'TYPE_BUFFS',  gradient: ['#120a20','#4a3070'] },
  // ... remaining TYPE_BUFFS categories
};
```

The dynamic slot uses `CATEGORY_META` to look up display name, gradient, and optional icon path for whichever category it is currently assigned.

### Dynamic Buff Slot (new)

- UUID: `com.battlestream.streamdeck.buff-slot`
- File: `streamdeck-plugin/src/actions/buff-slot.ts`
- The user places any number of these on their deck. Each self-identifies by its deck coordinates.

**What it shows:**
- Assigned + non-zero: category display name as label, `+ATK/+HP` as value, gradient and icon matching the assigned category (uses existing icon if one exists, else group gradient)
- Unassigned: black background, dim "BUFF" label, `—` value

**Assignment algorithm** (runs on every state update):

1. Build the set of active categories: all TARGETED + TYPE BUFFS entries in `buff_sources` where `attack != 0 || health != 0`.
2. For each active category already assigned to a slot: update that slot's `lastUpdated` timestamp.
3. For each active category not yet assigned:
   a. Find the first free slot ordered by deck position (row-major: `row * 1000 + col`).
   b. If no free slot exists: evict the slot with the oldest `lastUpdated` timestamp.
   c. Assign the category to the chosen slot; set `lastUpdated` to now.
4. For each slot whose assigned category is no longer active (dropped to 0): clear the assignment (slot becomes free).
5. Re-render all slots.

**State held by `DynamicBuffSlotAction` (singleton instance):**

```typescript
interface SlotState {
  category: string;   // assigned category key, e.g. "UNDEAD"
  lastUpdated: number; // Date.now() of last non-zero update
}

private slots = new Map<string, SlotState>();       // contextId → assignment
private coords = new Map<string, {row: number, col: number}>(); // contextId → position
```

**Position detection:** `action.coordinates` from the `@elgato/streamdeck` SDK on `onWillAppear`.

**Gradient per category when assigned:**
- TARGETED categories (no existing icon): gold `['#1a1000', '#7a5000']`
- TYPE BUFFS categories (no existing icon): purple `['#120a20', '#4a3070']`
- Categories with existing icons: use the existing icon + that action's gradient

### Profiles

The four bundled deck profiles (Standard, Mini, XL, Plus) should be updated to replace removed actions with the new Tavern-Wide Buff static button and a reasonable number of pre-placed Buff Slot buttons. Exact layout is left to implementation.

---

## Files Changed

### Go / TUI
- `internal/gamestate/categories.go` — add group constants + `CategoryGroup` map
- `internal/tui/tui.go` — rewrite `modsItems()` to render by group

### Stream Deck Plugin
- `src/actions/buff-atk.ts` — **deleted**
- `src/actions/buff-hp.ts` — **deleted**
- `src/actions/tavern-spell-buff.ts` — **deleted**
- `src/actions/auto-layout.ts` — **deleted**
- `src/actions/tavern-wide-buff.ts` — new
- `src/actions/bg-barrage-buff.ts` — new
- `src/actions/rightmost-buff.ts` — new
- `src/actions/nomi-buff.ts` — new
- `src/actions/undead-buff.ts` — new
- `src/actions/lightfang-buff.ts` — new
- `src/actions/whelp-buff.ts` — new
- `src/actions/beetle-buff.ts` — new
- `src/actions/volumizer-buff.ts` — new
- `src/actions/consumed-buff.ts` — new
- `src/actions/buff-slot.ts` — new (dynamic slot)
- `src/categories.ts` — new shared category metadata module
- `src/plugin.ts` — update imports and registrations
- `manifest.json` (in dist) — add new UUIDs, remove deleted ones
- `__tests__/actions/buff-slot.test.ts` — new unit tests for assignment algorithm

### Icons
- `imgs/actions/tavern-wide-buff.png` — repurpose or replace `buff-atk.png`
- No new icons needed for gradient-only buttons (gradient rendered at runtime)
