# Buff Grouping Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorganise buff display into three semantic groups (TAVERN-WIDE, TARGETED, TYPE BUFFS) in both the TUI and Stream Deck plugin, replacing the flat sorted list and the stale per-stat buttons.

**Architecture:** Pure display change — no proto/API/state changes. Go side adds a `CategoryGroup` map to `categories.go` and rewrites `modsItems()` in `tui.go`. Stream Deck side adds a shared `categories.ts` metadata module, 11 new static action files, a new `DynamicBuffSlotAction`, and removes 4 stale actions.

**Tech Stack:** Go 1.24, lipgloss (TUI), TypeScript 5 + `@elgato/streamdeck` SDK v2, Rollup, Jest

**Spec:** `docs/superpowers/specs/2026-05-08-buff-grouping-design.md`

**Note:** Parts 1 (Go/TUI) and 2 (Stream Deck) are fully independent — they share no code and can be worked in parallel or in any order.

---

## Part 1 — Go / TUI

### Task 1: Add CategoryGroup map to categories.go

**Files:**
- Modify: `internal/gamestate/categories.go` (after the existing `CategoryDisplayName` map)
- Modify: `internal/tui/tui_test.go` (add test for the new group helper — see Task 2 note)
- Test via: `internal/gamestate/categories_test.go` (add new test function)

- [ ] **Step 1: Write the failing test**

Add to `internal/gamestate/categories_test.go` (create the file if it doesn't exist — check first with `ls internal/gamestate/`):

```go
func TestCategoryGroup_AllCategoriesMapped(t *testing.T) {
    cases := []struct {
        cat   string
        group string
    }{
        {CatNomiAll, GroupTavernWide},
        {CatTavernSpell, GroupTavernWide},
        {CatShopBuff, GroupTavernWide},
        {CatGeneral, GroupTavernWide},
        {CatBloodgem, GroupTargeted},
        {CatBloodgemBarrage, GroupTargeted},
        {CatRightmost, GroupTargeted},
        {CatNomi, GroupTypeBuffs},
        {CatElemental, GroupTypeBuffs},
        {CatUndead, GroupTypeBuffs},
        {CatLightfang, GroupTypeBuffs},
        {CatWhelp, GroupTypeBuffs},
        {CatBeetle, GroupTypeBuffs},
        {CatVolumizer, GroupTypeBuffs},
        {CatConsumed, GroupTypeBuffs},
    }
    for _, tc := range cases {
        t.Run(tc.cat, func(t *testing.T) {
            got, ok := CategoryGroup[tc.cat]
            if !ok {
                t.Fatalf("CategoryGroup missing %q", tc.cat)
            }
            if got != tc.group {
                t.Errorf("CategoryGroup[%q] = %q, want %q", tc.cat, got, tc.group)
            }
        })
    }
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test -count=1 ./internal/gamestate/ -run TestCategoryGroup
```

Expected: `FAIL — undefined: GroupTavernWide`

- [ ] **Step 3: Add group constants and CategoryGroup map to categories.go**

Add after the closing `}` of `CategoryDisplayName` (around line 181):

```go
// Buff group constants used by the TUI and Stream Deck plugin to cluster
// categories by how they apply.
const (
    GroupTavernWide = "TAVERN_WIDE"
    GroupTargeted   = "TARGETED"
    GroupTypeBuffs  = "TYPE_BUFFS"
)

// CategoryGroup maps each buff category to its display group.
var CategoryGroup = map[string]string{
    CatNomiAll:         GroupTavernWide,
    CatTavernSpell:     GroupTavernWide,
    CatShopBuff:        GroupTavernWide,
    CatGeneral:         GroupTavernWide,

    CatBloodgem:        GroupTargeted,
    CatBloodgemBarrage: GroupTargeted,
    CatRightmost:       GroupTargeted,

    CatNomi:      GroupTypeBuffs,
    CatElemental: GroupTypeBuffs,
    CatUndead:    GroupTypeBuffs,
    CatLightfang: GroupTypeBuffs,
    CatWhelp:     GroupTypeBuffs,
    CatBeetle:    GroupTypeBuffs,
    CatVolumizer: GroupTypeBuffs,
    CatConsumed:  GroupTypeBuffs,
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test -count=1 ./internal/gamestate/ -run TestCategoryGroup
```

Expected: `PASS`

- [ ] **Step 5: Vet**

```bash
go vet ./internal/gamestate/
```

Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/gamestate/categories.go internal/gamestate/categories_test.go
git commit -m "feat(gamestate): add CategoryGroup map with three buff groups"
```

---

### Task 2: Rewrite modsItems() in tui.go

**Files:**
- Modify: `internal/tui/tui.go` — add color constant, add `groupedBuffs` struct + `groupBuffSources()` helper, rewrite `modsItems()`
- Modify: `internal/tui/tui_test.go` — add `TestGroupBuffSources` and `TestModsItems_GroupedSections`

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/tui_test.go` (the file already exists with `package tui` and `bspb` import):

```go
func TestGroupBuffSources(t *testing.T) {
    sources := []*bspb.BuffSource{
        {Category: "NOMI_ALL", Attack: 4, Health: 4},     // TAVERN_WIDE
        {Category: "TAVERN_SPELL", Attack: 8, Health: 4}, // TAVERN_WIDE
        {Category: "BLOODGEM", Attack: 3, Health: 0},     // TARGETED, non-zero
        {Category: "RIGHTMOST", Attack: 0, Health: 0},    // TARGETED, zero — excluded
        {Category: "UNDEAD", Attack: 6, Health: 6},       // TYPE_BUFFS, non-zero
        {Category: "NOMI", Attack: 0, Health: 0},         // TYPE_BUFFS, zero — excluded
    }

    g := groupBuffSources(sources)

    if g.tavernWideATK != 12 || g.tavernWideHP != 8 {
        t.Errorf("tavernWide = +%d/+%d, want +12/+8", g.tavernWideATK, g.tavernWideHP)
    }
    if len(g.targeted) != 1 || g.targeted[0].Category != "BLOODGEM" {
        t.Errorf("targeted = %v, want [BLOODGEM]", g.targeted)
    }
    if len(g.typeBuffs) != 1 || g.typeBuffs[0].Category != "UNDEAD" {
        t.Errorf("typeBuffs = %v, want [UNDEAD]", g.typeBuffs)
    }
}

func TestModsItems_GroupedSections(t *testing.T) {
    m := &Model{
        connState: stateConnected,
        game: &bspb.GameState{
            BuffSources: []*bspb.BuffSource{
                {Category: "NOMI_ALL", Attack: 6, Health: 6},
                {Category: "BLOODGEM", Attack: 4, Health: 0},
                {Category: "UNDEAD", Attack: 3, Health: 3},
            },
        },
    }

    out := m.modsItems()

    if !strings.Contains(out, "TAVERN-WIDE") {
        t.Error("expected TAVERN-WIDE section header")
    }
    if !strings.Contains(out, "+6/+6") {
        t.Error("expected tavern-wide total +6/+6")
    }
    if !strings.Contains(out, "TARGETED") {
        t.Error("expected TARGETED section header")
    }
    if !strings.Contains(out, "BLOODGEM") || !strings.Contains(out, "+4/+0") {
        t.Error("expected BLOODGEM +4/+0 in targeted section")
    }
    if !strings.Contains(out, "TYPE BUFFS") {
        t.Error("expected TYPE BUFFS section header")
    }
    if !strings.Contains(out, "Undead") || !strings.Contains(out, "+3/+3") {
        t.Error("expected Undead +3/+3 in type buffs section")
    }
}

func TestModsItems_EmptySectionsOmitted(t *testing.T) {
    m := &Model{
        connState: stateConnected,
        game: &bspb.GameState{
            BuffSources: []*bspb.BuffSource{
                {Category: "BLOODGEM", Attack: 2, Health: 0},
            },
        },
    }

    out := m.modsItems()

    if strings.Contains(out, "TAVERN-WIDE") {
        t.Error("expected TAVERN-WIDE section to be omitted when empty")
    }
    if strings.Contains(out, "TYPE BUFFS") {
        t.Error("expected TYPE BUFFS section to be omitted when empty")
    }
    if !strings.Contains(out, "TARGETED") {
        t.Error("expected TARGETED section to be present")
    }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test -count=1 ./internal/tui/ -run "TestGroupBuffSources|TestModsItems"
```

Expected: `FAIL — undefined: groupBuffSources`

- [ ] **Step 3: Add color constant and groupBuffSources helper to tui.go**

In the color constants block (around line 28), add after `colorGeneral`:

```go
colorGroupTavernWide = lipgloss.Color("51") // bright cyan for TAVERN-WIDE header
```

After the closing brace of the `modsItems` function (or anywhere in the file as a package-level function), add:

```go
type groupedBuffs struct {
    tavernWideATK int32
    tavernWideHP  int32
    targeted      []*bspb.BuffSource // non-zero only
    typeBuffs     []*bspb.BuffSource // non-zero only
}

func groupBuffSources(sources []*bspb.BuffSource) groupedBuffs {
    var g groupedBuffs
    for _, bs := range sources {
        switch gamestate.CategoryGroup[bs.Category] {
        case gamestate.GroupTavernWide:
            g.tavernWideATK += bs.Attack
            g.tavernWideHP += bs.Health
        case gamestate.GroupTargeted:
            if bs.Attack != 0 || bs.Health != 0 {
                g.targeted = append(g.targeted, bs)
            }
        case gamestate.GroupTypeBuffs:
            if bs.Attack != 0 || bs.Health != 0 {
                g.typeBuffs = append(g.typeBuffs, bs)
            }
        }
    }
    return g
}
```

- [ ] **Step 4: Replace modsItems() with the grouped implementation**

Replace the entire `modsItems()` function (lines 800–858) with:

```go
// modsItems returns the scrollable buff-sources content (no outer title).
func (m *Model) modsItems() string {
    var b strings.Builder
    if m.game == nil || len(m.game.BuffSources) == 0 {
        if m.game != nil && len(m.game.Modifications) > 0 {
            for _, mod := range m.game.Modifications {
                sign := "+"
                if mod.Delta < 0 {
                    sign = ""
                }
                line := fmt.Sprintf("T%-2d %s%d %-6s %s",
                    mod.Turn, sign, mod.Delta, mod.Stat, mod.Target)
                b.WriteString(styleMod.Render(line) + "\n")
            }
        } else {
            b.WriteString(styleDim.Render("(none this game)"))
        }
        return b.String()
    }

    g := groupBuffSources(m.game.BuffSources)

    // TAVERN-WIDE: single accumulated total
    if g.tavernWideATK != 0 || g.tavernWideHP != 0 {
        hdr := lipgloss.NewStyle().Foreground(colorGroupTavernWide).Bold(true)
        val := lipgloss.NewStyle().Foreground(colorGroupTavernWide)
        b.WriteString(hdr.Render("TAVERN-WIDE") + "\n")
        b.WriteString(val.Render(fmt.Sprintf("  +%d/+%d", g.tavernWideATK, g.tavernWideHP)) + "\n\n")
    }

    // TARGETED: per-category
    if len(g.targeted) > 0 {
        hdr := lipgloss.NewStyle().Foreground(colorGold).Bold(true)
        b.WriteString(hdr.Render("TARGETED") + "\n")
        for _, bs := range g.targeted {
            name := buffCategoryDisplayName(bs.Category)
            color := buffCategoryColor(bs.Category)
            line := fmt.Sprintf("  %-12s +%d/+%d", name, bs.Attack, bs.Health)
            b.WriteString(lipgloss.NewStyle().Foreground(color).Render(line) + "\n")
        }
        b.WriteString("\n")
    }

    // TYPE BUFFS: per-category
    if len(g.typeBuffs) > 0 {
        hdr := lipgloss.NewStyle().Foreground(colorTavern).Bold(true)
        b.WriteString(hdr.Render("TYPE BUFFS") + "\n")
        for _, bs := range g.typeBuffs {
            name := buffCategoryDisplayName(bs.Category)
            color := buffCategoryColor(bs.Category)
            line := fmt.Sprintf("  %-12s +%d/+%d", name, bs.Attack, bs.Health)
            b.WriteString(lipgloss.NewStyle().Foreground(color).Render(line) + "\n")
        }
        b.WriteString("\n")
    }

    // ABILITIES
    if m.game != nil && len(m.game.AbilityCounters) > 0 {
        b.WriteString("\n" + styleTitle.Render("ABILITIES") + "\n")
        for _, ac := range m.game.AbilityCounters {
            name := buffCategoryDisplayName(ac.Category)
            color := buffCategoryColor(ac.Category)
            style := lipgloss.NewStyle().Foreground(color)
            line := fmt.Sprintf("  %-12s %s", name, ac.Display)
            b.WriteString(style.Render(line) + "\n")
        }
    }

    return b.String()
}
```

- [ ] **Step 5: Run tests to confirm they pass**

```bash
go test -count=1 ./internal/tui/ -run "TestGroupBuffSources|TestModsItems"
```

Expected: `PASS`

- [ ] **Step 6: Run full test suite**

```bash
go test -count=1 ./...
```

Expected: all pass.

- [ ] **Step 7: Visual verification with --dump**

```bash
go build ./cmd/battlestream && ./battlestream tui --dump --width 120
```

Expected: BUFF SOURCES panel shows three section headers (TAVERN-WIDE, TARGETED, TYPE BUFFS) based on whatever game data is present. If no game is active you'll see `(none this game)`.

To test with synthetic data, inject a quick reparse or check the test output from `TestModsItems_GroupedSections` — the string `out` will contain the rendered sections.

- [ ] **Step 8: Vet and commit**

```bash
go vet ./...
git add internal/gamestate/categories.go internal/gamestate/categories_test.go \
        internal/tui/tui.go internal/tui/tui_test.go
git commit -m "feat(tui): group buff sources into TAVERN-WIDE / TARGETED / TYPE BUFFS"
```

---

## Part 2 — Stream Deck Plugin

All commands in this section run from `streamdeck-plugin/`.

### Task 3: Add shared categories.ts + tests

**Files:**
- Create: `streamdeck-plugin/src/categories.ts`
- Create: `streamdeck-plugin/src/__tests__/categories.test.ts`

- [ ] **Step 1: Write the failing test**

Create `streamdeck-plugin/src/__tests__/categories.test.ts`:

```typescript
import { CATEGORY_META, DYNAMIC_CATEGORIES, TAVERN_WIDE_CATEGORIES } from '../categories.js';

test('TAVERN_WIDE_CATEGORIES has exactly 4 entries', () => {
  expect(TAVERN_WIDE_CATEGORIES.size).toBe(4);
  expect(TAVERN_WIDE_CATEGORIES.has('NOMI_ALL')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('TAVERN_SPELL')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('SHOP_BUFF')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('GENERAL')).toBe(true);
});

test('DYNAMIC_CATEGORIES excludes all TAVERN_WIDE categories', () => {
  for (const cat of TAVERN_WIDE_CATEGORIES) {
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(false);
  }
});

test('DYNAMIC_CATEGORIES includes all TARGETED categories', () => {
  expect(DYNAMIC_CATEGORIES.has('BLOODGEM')).toBe(true);
  expect(DYNAMIC_CATEGORIES.has('BLOODGEM_BARRAGE')).toBe(true);
  expect(DYNAMIC_CATEGORIES.has('RIGHTMOST')).toBe(true);
});

test('DYNAMIC_CATEGORIES includes all TYPE_BUFFS categories', () => {
  for (const cat of ['NOMI', 'ELEMENTAL', 'UNDEAD', 'LIGHTFANG', 'WHELP', 'BEETLE', 'VOLUMIZER', 'CONSUMED']) {
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(true);
  }
});

test('every CATEGORY_META entry has displayName, group, and gradient', () => {
  for (const [cat, meta] of Object.entries(CATEGORY_META)) {
    expect(typeof meta.displayName).toBe('string');
    expect(meta.gradient).toHaveLength(2);
    expect(['TARGETED', 'TYPE_BUFFS']).toContain(meta.group);
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(true);
  }
});
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd streamdeck-plugin && npm test -- --testPathPattern=categories
```

Expected: `FAIL — Cannot find module '../categories.js'`

- [ ] **Step 3: Create src/categories.ts**

```typescript
import path from 'node:path';

export type BuffGroup = 'TAVERN_WIDE' | 'TARGETED' | 'TYPE_BUFFS';

export interface CategoryMeta {
  displayName: string;
  group: 'TARGETED' | 'TYPE_BUFFS';
  gradient: readonly [string, string];
  iconFile?: string; // filename under imgs/actions/ if a matching icon exists
}

// Categories in the TAVERN-WIDE group — handled by the static TavernWideBuffAction,
// never assigned to dynamic slots.
export const TAVERN_WIDE_CATEGORIES = new Set([
  'NOMI_ALL', 'TAVERN_SPELL', 'SHOP_BUFF', 'GENERAL',
]);

// Per-category metadata for TARGETED and TYPE_BUFFS categories.
// Used by static buff buttons and the dynamic buff slot.
export const CATEGORY_META: Record<string, CategoryMeta> = {
  // TARGETED
  BLOODGEM:         { displayName: 'Bloodgems',  group: 'TARGETED',   gradient: ['#3a1a00', '#e67e22'], iconFile: 'bloodgem-buff.png' },
  BLOODGEM_BARRAGE: { displayName: 'BG Barrage', group: 'TARGETED',   gradient: ['#1a1000', '#7a5000'] },
  RIGHTMOST:        { displayName: 'Rightmost',  group: 'TARGETED',   gradient: ['#1a1000', '#7a5000'] },
  // TYPE_BUFFS
  NOMI:             { displayName: 'Nomi',       group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  ELEMENTAL:        { displayName: 'Elementals', group: 'TYPE_BUFFS', gradient: ['#3a2a00', '#f39c12'], iconFile: 'elemental-buff.png' },
  UNDEAD:           { displayName: 'Undead',     group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  LIGHTFANG:        { displayName: 'Lightfang',  group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  WHELP:            { displayName: 'Whelps',     group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  BEETLE:           { displayName: 'Beetles',    group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  VOLUMIZER:        { displayName: 'Volumizer',  group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  CONSUMED:         { displayName: 'Consumed',   group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
};

// All categories that can appear in dynamic slots (TARGETED + TYPE_BUFFS).
export const DYNAMIC_CATEGORIES = new Set(Object.keys(CATEGORY_META));

// Resolved path to the icon file for a category, or undefined if no icon exists.
export function categoryIconPath(cat: string): string | undefined {
  const file = CATEGORY_META[cat]?.iconFile;
  return file ? path.join(process.cwd(), 'imgs', 'actions', file) : undefined;
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
npm test -- --testPathPattern=categories
```

Expected: `PASS` (5 tests)

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/categories.ts streamdeck-plugin/src/__tests__/categories.test.ts
git commit -m "feat(streamdeck): add shared category metadata module"
```

---

### Task 4: Add tavern-wide-buff static action + copy icon

**Files:**
- Create: `streamdeck-plugin/src/actions/tavern-wide-buff.ts`
- Copy icon: `streamdeck-plugin/imgs/actions/tavern-wide-buff.png` (copy from `buff-atk.png`)

- [ ] **Step 1: Copy icon**

```bash
cp streamdeck-plugin/imgs/actions/buff-atk.png streamdeck-plugin/imgs/actions/tavern-wide-buff.png
```

- [ ] **Step 2: Create src/actions/tavern-wide-buff.ts**

```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import { TAVERN_WIDE_CATEGORIES } from '../categories.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.tavern-wide-buff' })
export class TavernWideBuffAction extends BaseStat {
  label = 'TVN WIDE';
  gradient = ['#001a26', '#1a6b8a'] as const;

  extract(s: GameState) {
    let atk = 0, hp = 0;
    for (const bs of s.buff_sources ?? []) {
      if (TAVERN_WIDE_CATEGORIES.has(bs.category)) {
        atk += bs.attack;
        hp += bs.health;
      }
    }
    return { value: `+${atk}/+${hp}`, subtitle: '' };
  }
}
```

- [ ] **Step 3: Add test to static-buffs.test.ts (create the file)**

Create `streamdeck-plugin/src/__tests__/actions/static-buffs.test.ts`:

```typescript
jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => Promise.resolve('data:image/png;base64,FAKE')),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));

import type { GameState } from '../../types.js';
import { TavernWideBuffAction } from '../../actions/tavern-wide-buff.js';

const base: GameState = {
  game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
  player: {} as never, board: [], placement: 0,
  buff_sources: [], ability_counters: [], anomaly_name: '', is_duos: false,
};

describe('TavernWideBuffAction', () => {
  test('sums NOMI_ALL + TAVERN_SPELL + SHOP_BUFF + GENERAL, excludes others', () => {
    const a = new TavernWideBuffAction();
    const s: GameState = { ...base, buff_sources: [
      { category: 'NOMI_ALL',    attack: 4, health: 4 },
      { category: 'TAVERN_SPELL', attack: 8, health: 4 },
      { category: 'SHOP_BUFF',   attack: 2, health: 2 },
      { category: 'BLOODGEM',    attack: 3, health: 0 }, // excluded — not TAVERN_WIDE
    ]};
    expect(a.extract(s).value).toBe('+14/+10');
  });

  test('returns +0/+0 when no tavern-wide sources present', () => {
    const a = new TavernWideBuffAction();
    expect(a.extract(base).value).toBe('+0/+0');
  });
});
```

- [ ] **Step 4: Run test**

```bash
npm test -- --testPathPattern=static-buffs
```

Expected: `PASS` (2 tests)

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/actions/tavern-wide-buff.ts \
        streamdeck-plugin/imgs/actions/tavern-wide-buff.png \
        streamdeck-plugin/src/__tests__/actions/static-buffs.test.ts
git commit -m "feat(streamdeck): add TavernWideBuffAction static button"
```

---

### Task 5: Add targeted static buttons (bg-barrage-buff, rightmost-buff)

**Files:**
- Create: `streamdeck-plugin/src/actions/bg-barrage-buff.ts`
- Create: `streamdeck-plugin/src/actions/rightmost-buff.ts`
- Modify: `streamdeck-plugin/src/__tests__/actions/static-buffs.test.ts`

- [ ] **Step 1: Add failing tests to static-buffs.test.ts**

Append to `src/__tests__/actions/static-buffs.test.ts`:

```typescript
import { BgBarrageBuffAction } from '../../actions/bg-barrage-buff.js';
import { RightmostBuffAction } from '../../actions/rightmost-buff.js';

describe('BgBarrageBuffAction', () => {
  test('returns +ATK/+HP for BLOODGEM_BARRAGE category', () => {
    const a = new BgBarrageBuffAction();
    const s: GameState = { ...base, buff_sources: [{ category: 'BLOODGEM_BARRAGE', attack: 3, health: 2 }] };
    expect(a.extract(s).value).toBe('+3/+2');
  });
  test('returns +0/+0 when category absent', () => {
    expect(new BgBarrageBuffAction().extract(base).value).toBe('+0/+0');
  });
});

describe('RightmostBuffAction', () => {
  test('returns +ATK/+HP for RIGHTMOST category', () => {
    const a = new RightmostBuffAction();
    const s: GameState = { ...base, buff_sources: [{ category: 'RIGHTMOST', attack: 2, health: 1 }] };
    expect(a.extract(s).value).toBe('+2/+1');
  });
  test('returns +0/+0 when category absent', () => {
    expect(new RightmostBuffAction().extract(base).value).toBe('+0/+0');
  });
});
```

- [ ] **Step 2: Run to confirm fail**

```bash
npm test -- --testPathPattern=static-buffs
```

Expected: `FAIL — Cannot find module '../../actions/bg-barrage-buff.js'`

- [ ] **Step 3: Create src/actions/bg-barrage-buff.ts**

```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.bg-barrage-buff' })
export class BgBarrageBuffAction extends BaseStat {
  label = 'BG BARRAGE';
  gradient = ['#1a1000', '#7a5000'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'BLOODGEM_BARRAGE');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

- [ ] **Step 4: Create src/actions/rightmost-buff.ts**

```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.rightmost-buff' })
export class RightmostBuffAction extends BaseStat {
  label = 'RIGHTMOST';
  gradient = ['#1a1000', '#7a5000'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'RIGHTMOST');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

- [ ] **Step 5: Run tests to confirm pass**

```bash
npm test -- --testPathPattern=static-buffs
```

Expected: `PASS` (6 tests)

- [ ] **Step 6: Commit**

```bash
git add streamdeck-plugin/src/actions/bg-barrage-buff.ts \
        streamdeck-plugin/src/actions/rightmost-buff.ts \
        streamdeck-plugin/src/__tests__/actions/static-buffs.test.ts
git commit -m "feat(streamdeck): add BgBarrage and Rightmost targeted buff buttons"
```

---

### Task 6: Add all Type Buffs static buttons (8 actions)

**Files:**
- Create: `src/actions/nomi-buff.ts`, `undead-buff.ts`, `lightfang-buff.ts`, `whelp-buff.ts`, `beetle-buff.ts`, `volumizer-buff.ts`, `consumed-buff.ts`
- Note: `elemental-buff.ts` already exists and needs no changes.
- Modify: `src/__tests__/actions/static-buffs.test.ts`

- [ ] **Step 1: Add failing tests to static-buffs.test.ts**

Append to `src/__tests__/actions/static-buffs.test.ts`:

```typescript
import { NomiBuffAction }      from '../../actions/nomi-buff.js';
import { UndeadBuffAction }    from '../../actions/undead-buff.js';
import { LightfangBuffAction } from '../../actions/lightfang-buff.js';
import { WhelpBuffAction }     from '../../actions/whelp-buff.js';
import { BeetleBuffAction }    from '../../actions/beetle-buff.js';
import { VolumizerBuffAction } from '../../actions/volumizer-buff.js';
import { ConsumedBuffAction }  from '../../actions/consumed-buff.js';

type MakeAction = () => { extract(s: GameState): { value: string } };
const TYPE_BUFF_CASES: Array<[string, MakeAction, string]> = [
  ['NomiBuffAction',      () => new NomiBuffAction(),      'NOMI'],
  ['UndeadBuffAction',    () => new UndeadBuffAction(),    'UNDEAD'],
  ['LightfangBuffAction', () => new LightfangBuffAction(), 'LIGHTFANG'],
  ['WhelpBuffAction',     () => new WhelpBuffAction(),     'WHELP'],
  ['BeetleBuffAction',    () => new BeetleBuffAction(),    'BEETLE'],
  ['VolumizerBuffAction', () => new VolumizerBuffAction(), 'VOLUMIZER'],
  ['ConsumedBuffAction',  () => new ConsumedBuffAction(),  'CONSUMED'],
];

test.each(TYPE_BUFF_CASES)('%s returns +ATK/+HP for its category', (_name, makeAction, cat) => {
  const a = makeAction();
  const s: GameState = { ...base, buff_sources: [{ category: cat, attack: 4, health: 2 }] };
  expect(a.extract(s).value).toBe('+4/+2');
});

test.each(TYPE_BUFF_CASES)('%s returns +0/+0 when category absent', (_name, makeAction) => {
  expect(makeAction().extract(base).value).toBe('+0/+0');
});
```

- [ ] **Step 2: Run to confirm fail**

```bash
npm test -- --testPathPattern=static-buffs
```

Expected: `FAIL — Cannot find module '../../actions/nomi-buff.js'`

- [ ] **Step 3: Create all 7 type buff action files**

Each file follows the same pattern. Create all 7:

**`src/actions/nomi-buff.ts`:**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.nomi-buff' })
export class NomiBuffAction extends BaseStat {
  label = 'NOMI';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'NOMI');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`src/actions/undead-buff.ts`:**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.undead-buff' })
export class UndeadBuffAction extends BaseStat {
  label = 'UNDEAD';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'UNDEAD');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`src/actions/lightfang-buff.ts`:**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.lightfang-buff' })
export class LightfangBuffAction extends BaseStat {
  label = 'LIGHTFANG';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'LIGHTFANG');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`src/actions/whelp-buff.ts`:**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.whelp-buff' })
export class WhelpBuffAction extends BaseStat {
  label = 'WHELPS';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'WHELP');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`src/actions/beetle-buff.ts`:**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.beetle-buff' })
export class BeetleBuffAction extends BaseStat {
  label = 'BEETLES';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'BEETLE');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`src/actions/volumizer-buff.ts`:**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.volumizer-buff' })
export class VolumizerBuffAction extends BaseStat {
  label = 'VOLUMIZER';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'VOLUMIZER');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`src/actions/consumed-buff.ts`:**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.consumed-buff' })
export class ConsumedBuffAction extends BaseStat {
  label = 'CONSUMED';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'CONSUMED');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

- [ ] **Step 4: Run tests to confirm pass**

```bash
npm test -- --testPathPattern=static-buffs
```

Expected: `PASS` (all tests including the 14 table-driven cases)

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/actions/nomi-buff.ts \
        streamdeck-plugin/src/actions/undead-buff.ts \
        streamdeck-plugin/src/actions/lightfang-buff.ts \
        streamdeck-plugin/src/actions/whelp-buff.ts \
        streamdeck-plugin/src/actions/beetle-buff.ts \
        streamdeck-plugin/src/actions/volumizer-buff.ts \
        streamdeck-plugin/src/actions/consumed-buff.ts \
        streamdeck-plugin/src/__tests__/actions/static-buffs.test.ts
git commit -m "feat(streamdeck): add 7 Type Buffs static buff buttons"
```

---

### Task 7: Add DynamicBuffSlotAction (buff-slot.ts) + tests

**Files:**
- Create: `streamdeck-plugin/src/actions/buff-slot.ts`
- Create: `streamdeck-plugin/src/__tests__/actions/buff-slot.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `streamdeck-plugin/src/__tests__/actions/buff-slot.test.ts`:

```typescript
jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => Promise.resolve('data:image/png;base64,FAKE')),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));
jest.mock('../../state.js', () => ({
  store: { subscribe: jest.fn(() => () => {}), getState: jest.fn(() => null) },
}));

import { DynamicBuffSlotAction } from '../../actions/buff-slot.js';
import type { GameState } from '../../types.js';

function makeAction(id: string, row = 0, col = 0) {
  return {
    id,
    coordinates: { row, column: col },
    setImage: jest.fn().mockResolvedValue(undefined),
  };
}

async function appear(inst: DynamicBuffSlotAction, ...actions: ReturnType<typeof makeAction>[]) {
  for (const a of actions) {
    await inst.onWillAppear({ action: a } as never);
  }
}

function makeState(...sources: Array<{ category: string; attack: number; health: number }>): GameState {
  return {
    game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
    player: {} as never, board: [], placement: 0,
    buff_sources: sources, ability_counters: [], anomaly_name: '', is_duos: false,
  };
}

describe('DynamicBuffSlotAction.assign()', () => {
  test('assigns first active category to position-sorted first slot', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0), makeAction('ctx-2', 0, 1));

    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));

    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    expect(inst.getSlots().has('ctx-2')).toBe(false);
  });

  test('fills multiple slots in row-major position order', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0), makeAction('ctx-2', 0, 1));

    inst.assign(makeState(
      { category: 'UNDEAD', attack: 4, health: 4 },
      { category: 'NOMI',   attack: 2, health: 2 },
    ));

    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    expect(inst.getSlots().get('ctx-2')?.category).toBe('NOMI');
  });

  test('slot at row 1 is filled after all row 0 slots', async () => {
    const inst = new DynamicBuffSlotAction();
    // Register in reverse order — order of appearance must not affect assignment
    await appear(inst, makeAction('ctx-row1', 1, 0), makeAction('ctx-row0', 0, 2));

    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));

    expect(inst.getSlots().get('ctx-row0')?.category).toBe('UNDEAD');
    expect(inst.getSlots().has('ctx-row1')).toBe(false);
  });

  test('evicts least-recently-updated slot when all slots full', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));

    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    // Backdate to simulate stale
    inst.getSlots().get('ctx-1')!.lastUpdated = 1000;

    inst.assign(makeState(
      { category: 'UNDEAD', attack: 4, health: 4 },
      { category: 'NOMI',   attack: 2, health: 2 },
    ));

    expect(inst.getSlots().get('ctx-1')?.category).toBe('NOMI');
  });

  test('clears slot when assigned category drops to 0', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));

    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');

    inst.assign(makeState({ category: 'UNDEAD', attack: 0, health: 0 }));
    expect(inst.getSlots().has('ctx-1')).toBe(false);
  });

  test('TAVERN_WIDE categories are never assigned to slots', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));

    inst.assign(makeState(
      { category: 'NOMI_ALL',    attack: 6, health: 6 },
      { category: 'TAVERN_SPELL', attack: 4, health: 2 },
      { category: 'SHOP_BUFF',   attack: 2, health: 2 },
    ));

    expect(inst.getSlots().has('ctx-1')).toBe(false);
  });

  test('null state clears all slot assignments', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));

    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');

    inst.assign(null);
    expect(inst.getSlots().size).toBe(0);
  });
});
```

- [ ] **Step 2: Run to confirm fail**

```bash
npm test -- --testPathPattern=buff-slot
```

Expected: `FAIL — Cannot find module '../../actions/buff-slot.js'`

- [ ] **Step 3: Create src/actions/buff-slot.ts**

```typescript
import { action, SingletonAction, type WillAppearEvent, type WillDisappearEvent } from '@elgato/streamdeck';
import path from 'node:path';
import { store } from '../state.js';
import { renderButton } from '../render.js';
import { CATEGORY_META, DYNAMIC_CATEGORIES, categoryIconPath } from '../categories.js';
import type { GameState } from '../types.js';

export interface SlotState {
  category: string;
  lastUpdated: number;
}

interface ActionLike {
  id: string;
  setImage(image: string): Promise<void>;
  coordinates?: { row: number; column: number };
}

@action({ UUID: 'com.battlestream.streamdeck.buff-slot' })
export class DynamicBuffSlotAction extends SingletonAction<Record<string, never>> {
  private readonly slots   = new Map<string, SlotState>();                    // contextId → assignment
  private readonly coords  = new Map<string, { row: number; col: number }>(); // contextId → deck position
  private readonly actions = new Map<string, ActionLike>();                   // contextId → SDK action
  private unsub?: () => void;

  // Public for testing
  getSlots(): Map<string, SlotState> { return this.slots; }

  override async onWillAppear({ action }: WillAppearEvent<Record<string, never>>): Promise<void> {
    const a = action as unknown as ActionLike;
    const { row = 0, column: col = 0 } = a.coordinates ?? {};
    this.coords.set(a.id, { row, col });
    this.actions.set(a.id, a);

    if (this.actions.size === 1) {
      this.unsub = store.subscribe(state => void this.onStateUpdate(state));
    }

    await this.onStateUpdate(store.getState());
  }

  override async onWillDisappear({ action }: WillDisappearEvent<Record<string, never>>): Promise<void> {
    const a = action as unknown as ActionLike;
    this.slots.delete(a.id);
    this.coords.delete(a.id);
    this.actions.delete(a.id);

    if (this.actions.size === 0) {
      this.unsub?.();
      this.unsub = undefined;
    }
  }

  private async onStateUpdate(state: GameState | null): Promise<void> {
    this.assign(state);
    await this.renderAll(state);
  }

  // Public for testing — pure assignment logic with no rendering side-effects.
  assign(state: GameState | null): void {
    if (state === null) {
      this.slots.clear();
      return;
    }

    const now = Date.now();

    // Build set of active (non-zero) dynamic categories from current state.
    const active = new Map<string, true>();
    for (const bs of state.buff_sources ?? []) {
      if (DYNAMIC_CATEGORIES.has(bs.category) && (bs.attack !== 0 || bs.health !== 0)) {
        active.set(bs.category, true);
      }
    }

    // Step 1: clear slots whose category is no longer active.
    for (const [id, slot] of this.slots) {
      if (!active.has(slot.category)) this.slots.delete(id);
    }

    // Step 2: refresh lastUpdated for still-active assigned categories.
    for (const slot of this.slots.values()) {
      if (active.has(slot.category)) slot.lastUpdated = now;
    }

    // Step 3: assign newly-active categories to free or LRU slots.
    const assigned = new Set([...this.slots.values()].map(s => s.category));
    for (const cat of active.keys()) {
      if (assigned.has(cat)) continue;

      const sorted = this.sortedIds();
      const freeId = sorted.find(id => !this.slots.has(id));

      if (freeId !== undefined) {
        this.slots.set(freeId, { category: cat, lastUpdated: now });
      } else {
        // Evict the slot that was least recently updated.
        let lruId: string | undefined;
        let lruTime = Infinity;
        for (const [id, slot] of this.slots) {
          if (slot.lastUpdated < lruTime) { lruTime = slot.lastUpdated; lruId = id; }
        }
        if (lruId !== undefined) {
          this.slots.set(lruId, { category: cat, lastUpdated: now });
        }
      }
      assigned.add(cat);
    }
  }

  private sortedIds(): string[] {
    return [...this.actions.keys()].sort((a, b) => {
      const ca = this.coords.get(a) ?? { row: 0, col: 0 };
      const cb = this.coords.get(b) ?? { row: 0, col: 0 };
      return (ca.row * 1000 + ca.col) - (cb.row * 1000 + cb.col);
    });
  }

  private async renderAll(state: GameState | null): Promise<void> {
    await Promise.all([...this.actions.entries()].map(([id, a]) => this.renderOne(id, a, state)));
  }

  private async renderOne(id: string, a: ActionLike, state: GameState | null): Promise<void> {
    const slot = this.slots.get(id);

    if (!slot || state === null) {
      const img = await renderButton({
        label: 'BUFF', value: '—', subtitle: '',
        gradient: ['#000000', '#000000'],
        offline: state === null,
      });
      await a.setImage(img);
      return;
    }

    const meta = CATEGORY_META[slot.category];
    const bs   = state.buff_sources?.find(b => b.category === slot.category);
    const img  = await renderButton({
      label:    meta?.displayName ?? slot.category,
      value:    bs ? `+${bs.attack}/+${bs.health}` : '+0/+0',
      subtitle: '',
      gradient: meta?.gradient ?? ['#120a20', '#4a3070'],
      offline:  false,
      iconPath: categoryIconPath(slot.category),
    });
    await a.setImage(img);
  }
}
```

- [ ] **Step 4: Run tests to confirm pass**

```bash
npm test -- --testPathPattern=buff-slot
```

Expected: `PASS` (7 tests)

- [ ] **Step 5: Run full test suite**

```bash
npm test
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add streamdeck-plugin/src/actions/buff-slot.ts \
        streamdeck-plugin/src/__tests__/actions/buff-slot.test.ts
git commit -m "feat(streamdeck): add DynamicBuffSlotAction with LRU assignment"
```

---

### Task 8: Delete removed actions and update plugin.ts

**Files:**
- Delete: `src/actions/buff-atk.ts`, `buff-hp.ts`, `tavern-spell-buff.ts`, `auto-layout.ts`
- Delete: `src/__tests__/actions/auto-layout.test.ts`
- Modify: `src/plugin.ts`

- [ ] **Step 1: Delete the removed action files**

```bash
rm streamdeck-plugin/src/actions/buff-atk.ts \
   streamdeck-plugin/src/actions/buff-hp.ts \
   streamdeck-plugin/src/actions/tavern-spell-buff.ts \
   streamdeck-plugin/src/actions/auto-layout.ts \
   streamdeck-plugin/src/__tests__/actions/auto-layout.test.ts
```

- [ ] **Step 2: Replace plugin.ts**

Replace `streamdeck-plugin/src/plugin.ts` with:

```typescript
import streamDeck from '@elgato/streamdeck';
import { EventSource } from 'eventsource';
import { BattlestreamClient } from './client.js';
import { store } from './state.js';
import type { GlobalSettings } from './types.js';

import { HealthAction }         from './actions/health.js';
import { ArmorAction }          from './actions/armor.js';
import { TavernTierAction }     from './actions/tavern-tier.js';
import { GoldAction }           from './actions/gold.js';
import { TriplesAction }        from './actions/triples.js';
import { WinStreakAction }       from './actions/win-streak.js';
import { LossStreakAction }      from './actions/loss-streak.js';
import { PlacementAction }      from './actions/placement.js';
import { SpellPowerAction }     from './actions/spell-power.js';
import { TurnAction }           from './actions/turn.js';
import { PhaseAction }          from './actions/phase.js';
import { MinionCountAction }    from './actions/minion-count.js';
import { AnomalyAction }        from './actions/anomaly.js';
import { SpellcraftAction }     from './actions/spellcraft.js';
// Buff buttons
import { TavernWideBuffAction } from './actions/tavern-wide-buff.js';
import { BloodgemBuffAction }   from './actions/bloodgem-buff.js';
import { BgBarrageBuffAction }  from './actions/bg-barrage-buff.js';
import { RightmostBuffAction }  from './actions/rightmost-buff.js';
import { ElementalBuffAction }  from './actions/elemental-buff.js';
import { NomiBuffAction }       from './actions/nomi-buff.js';
import { UndeadBuffAction }     from './actions/undead-buff.js';
import { LightfangBuffAction }  from './actions/lightfang-buff.js';
import { WhelpBuffAction }      from './actions/whelp-buff.js';
import { BeetleBuffAction }     from './actions/beetle-buff.js';
import { VolumizerBuffAction }  from './actions/volumizer-buff.js';
import { ConsumedBuffAction }   from './actions/consumed-buff.js';
import { DynamicBuffSlotAction } from './actions/buff-slot.js';

streamDeck.actions.registerAction(new HealthAction());
streamDeck.actions.registerAction(new ArmorAction());
streamDeck.actions.registerAction(new TavernTierAction());
streamDeck.actions.registerAction(new GoldAction());
streamDeck.actions.registerAction(new TriplesAction());
streamDeck.actions.registerAction(new WinStreakAction());
streamDeck.actions.registerAction(new LossStreakAction());
streamDeck.actions.registerAction(new PlacementAction());
streamDeck.actions.registerAction(new SpellPowerAction());
streamDeck.actions.registerAction(new TurnAction());
streamDeck.actions.registerAction(new PhaseAction());
streamDeck.actions.registerAction(new MinionCountAction());
streamDeck.actions.registerAction(new AnomalyAction());
streamDeck.actions.registerAction(new SpellcraftAction());
// Buff buttons
streamDeck.actions.registerAction(new TavernWideBuffAction());
streamDeck.actions.registerAction(new BloodgemBuffAction());
streamDeck.actions.registerAction(new BgBarrageBuffAction());
streamDeck.actions.registerAction(new RightmostBuffAction());
streamDeck.actions.registerAction(new ElementalBuffAction());
streamDeck.actions.registerAction(new NomiBuffAction());
streamDeck.actions.registerAction(new UndeadBuffAction());
streamDeck.actions.registerAction(new LightfangBuffAction());
streamDeck.actions.registerAction(new WhelpBuffAction());
streamDeck.actions.registerAction(new BeetleBuffAction());
streamDeck.actions.registerAction(new VolumizerBuffAction());
streamDeck.actions.registerAction(new ConsumedBuffAction());
streamDeck.actions.registerAction(new DynamicBuffSlotAction());

function makeClient(host: string, port: number, apiKey: string): BattlestreamClient {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return new BattlestreamClient(
    { host, port, apiKey },
    { onState: state => store.setState(state), EventSourceImpl: EventSource as any },
  );
}

let client: BattlestreamClient | null = null;

function applySettings(settings: GlobalSettings): void {
  const host   = settings.host?.trim() || '127.0.0.1';
  const port   = settings.port ?? 8080;
  const apiKey = settings.apiKey ?? '';
  store.setSettings({ host, port, apiKey });
  client?.disconnect();
  client = makeClient(host, port, apiKey);
  client.connect();
}

streamDeck.settings.onDidReceiveGlobalSettings(({ settings }) => {
  applySettings(settings as GlobalSettings);
});

streamDeck.connect().then(() => {
  streamDeck.settings.getGlobalSettings();
});
```

- [ ] **Step 3: Run full test suite**

```bash
npm test
```

Expected: all pass (auto-layout test gone, all other tests still pass).

- [ ] **Step 4: Commit**

```bash
git add streamdeck-plugin/src/plugin.ts
git rm streamdeck-plugin/src/actions/buff-atk.ts \
       streamdeck-plugin/src/actions/buff-hp.ts \
       streamdeck-plugin/src/actions/tavern-spell-buff.ts \
       streamdeck-plugin/src/actions/auto-layout.ts \
       streamdeck-plugin/src/__tests__/actions/auto-layout.test.ts
git commit -m "feat(streamdeck): remove stale buff/auto-layout actions, wire new actions"
```

---

### Task 9: Update manifest.json

**Files:**
- Modify: `streamdeck-plugin/manifest.json`

- [ ] **Step 1: Remove 4 entries and add 12 new ones**

Open `streamdeck-plugin/manifest.json`. The `Actions` array currently has 20 entries. Make the following changes:

**Remove these 4 action objects** (match by UUID):
- `com.battlestream.streamdeck.buff-atk`
- `com.battlestream.streamdeck.buff-hp`
- `com.battlestream.streamdeck.tavern-spell-buff`
- `com.battlestream.streamdeck.auto-layout`

**Add these 11 new action objects** (append to the `Actions` array):

```json
{
  "UUID": "com.battlestream.streamdeck.tavern-wide-buff",
  "Name": "Tavern-Wide Buff",
  "Icon": "imgs/actions/tavern-wide-buff",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/actions/tavern-wide-buff" }]
},
{
  "UUID": "com.battlestream.streamdeck.bg-barrage-buff",
  "Name": "BG Barrage Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.rightmost-buff",
  "Name": "Rightmost Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.nomi-buff",
  "Name": "Nomi Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.undead-buff",
  "Name": "Undead Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.lightfang-buff",
  "Name": "Lightfang Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.whelp-buff",
  "Name": "Whelp Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.beetle-buff",
  "Name": "Beetle Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.volumizer-buff",
  "Name": "Volumizer Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.consumed-buff",
  "Name": "Consumed Buff",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
},
{
  "UUID": "com.battlestream.streamdeck.buff-slot",
  "Name": "Buff Slot",
  "Icon": "imgs/category",
  "Controllers": ["Keypad"],
  "States": [{ "Image": "imgs/category" }]
}
```

After editing, verify the Actions array has exactly 27 entries (20 − 4 + 11):

```bash
cat streamdeck-plugin/manifest.json | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d['Actions']), 'actions')"
```

Expected: `27 actions`

- [ ] **Step 2: Build the plugin to verify manifest is valid**

```bash
cd streamdeck-plugin && npm run build
```

Expected: build succeeds with no errors. `dist/com.battlestream.streamdeck.sdPlugin/` is updated.

- [ ] **Step 3: Commit**

```bash
git add streamdeck-plugin/manifest.json
git commit -m "feat(streamdeck): update manifest — add 12 new actions, remove 4 stale"
```

---

### Task 10: Build, run full tests, push

- [ ] **Step 1: Run all Go tests**

```bash
go test -count=1 ./...
```

Expected: all pass.

- [ ] **Step 2: Run all Stream Deck tests**

```bash
cd streamdeck-plugin && npm test
```

Expected: all pass.

- [ ] **Step 3: Run go vet**

```bash
go vet ./...
```

Expected: no output.

- [ ] **Step 4: Build the plugin one final time**

```bash
cd streamdeck-plugin && npm run build
```

Expected: succeeds.

- [ ] **Step 5: Final commit and push**

```bash
git push origin main
```

Then check CI:

```bash
gh run watch
```

Expected: all CI jobs green (Build & Test, Lint, Proto check, Docker build).

---

## Notes

**Stream Deck profiles:** The four `.sdProfile` files in `streamdeck-plugin/profiles/` are Zip archives that must be updated through the Stream Deck application (File → Import Profile, make changes, export). They cannot be edited as plain text. Update them separately using the Stream Deck UI: remove `Buff ATK`, `Buff HP`, `Tavern Spell Buff`, and `Auto-Layout` buttons; add `Tavern-Wide Buff` and several `Buff Slot` buttons in their place.

**elemental-buff.ts and bloodgem-buff.ts:** These two files are unchanged. They already implement the correct `extract()` pattern and their category strings (`ELEMENTAL`, `BLOODGEM`) are valid in the new scheme.
