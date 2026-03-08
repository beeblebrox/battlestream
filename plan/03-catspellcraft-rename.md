# 03 — [BUG] `CatSpellcraft` is a misnomer — tracks Naga spells, not Spellcraft keyword

**Priority:** CRITICAL
**Area:** `internal/gamestate/categories.go`, `internal/gamestate/processor.go`, TUI, docs

## Problem

The constant `CatSpellcraft` and its associated display label `"Spellcraft"` are incorrect:

- Tag `3809` is HDT's `SpellsPlayedForNagasCounter` — counts total spells cast this game
  for Naga minions whose permanent buff scales with spells played (Arcane Cannoneer,
  Thaumaturgist, Showy Cyclist, Groundbreaker).
- The Spellcraft *keyword* (gives a temporary spell each turn to Spellcraft minions) is
  a completely different mechanic and is not tracked by tag 3809.

The display formula `stacks (progress/4)` is also unexplained. It means:
- Current buff-per-spell tier = `stacks` (i.e., floor(total/4))
- Progress within current tier = `progress % 4`

Any user who knows what the Spellcraft keyword does will be confused by this label.

## Impact

Wrong labelling throughout: categories.go constant, processor handler, TUI ABILITIES panel,
and documentation. Misleads users and future contributors.

## Fixes needed

### 1. Rename the constant

`categories.go`:
```go
// Before
CatSpellcraft BuffCategory = "spellcraft"

// After
CatNagaSpells BuffCategory = "naga_spells"
```

### 2. Update `CategoryDisplayName`

```go
// Before
case CatSpellcraft: return "Spellcraft"

// After
case CatNagaSpells: return "Naga Spells"
```

### 3. Update all references in processor.go

Search for `CatSpellcraft` and replace with `CatNagaSpells`.

### 4. Clarify display format in TUI

Current: `N (M/4)` — opaque
Proposed: `Tier N · M/4` or add a sub-label: `"Naga Spells  Tier 2 · 3/4"`

### 5. Update documentation

- `docs/PARSER_SPEC.md` — remove/correct "Spellcraft stacks" description
- `docs/PARSER.md` — same
- `docs/TODO.md` — mark this item resolved once done

## Files to change

- `internal/gamestate/categories.go`
- `internal/gamestate/processor.go`
- `internal/tui/` — wherever `CatSpellcraft` is referenced for display
- `docs/PARSER_SPEC.md`, `docs/PARSER.md`

## Complexity

Low-medium — mechanical rename + display copy tweak. Use `grep -r CatSpellcraft` to find
all references before changing.

## Verification

- `./battlestream tui --dump` should show "Naga Spells" in the ABILITIES panel
- Grep confirms zero remaining references to `CatSpellcraft` or `"Spellcraft"` in Go source
