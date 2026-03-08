# 11 — [IMPROVEMENT] `Modifications[]` Source/Category/CardID always empty

**Priority:** MEDIUM
**Area:** `internal/gamestate/processor.go` — board-wide buff detection / `flushPendingStatChanges`

## Problem

`Modifications []StatMod` entries are populated with `Turn`, `Target`, `Stat`, and `Delta`
but `Source`, `Category`, and `CardID` are always left blank. The block source context
(`BlockSource`, `BlockCardID`) is available on the event at the time stat changes are
batched, but is not captured.

This means the REST/gRPC response and the TUI show which minions got buffed and by how
much, but not *what* caused the buff — the card/enchantment source is lost.

## Fix

### Step 1: Carry block context into `pendingStatChange`

When appending to `pendingStatChanges`, also store the current `p.blockSource` and
`p.blockCardID`:

```go
type pendingStatChange struct {
    entityID    int
    stat        string
    delta       int
    turn        int
    blockSource string
    blockCardID string
}
```

### Step 2: Use block context when building `StatMod`

In `flushPendingStatChanges`, when building the `StatMod` for a detected board-wide buff,
use the stored block context:

```go
mod := StatMod{
    Turn:     sc.turn,
    Target:   targetName,
    Stat:     sc.stat,
    Delta:    sc.delta,
    Source:   sc.blockSource,
    CardID:   sc.blockCardID,
    Category: categoryByEnchantmentCardID[sc.blockCardID],
}
```

### Step 3: Handle conflicting sources within a batch

If a batch of stat changes spans multiple block sources (unusual but possible), either
use the most common source in the batch or emit one `StatMod` per source group.

## Files to change

- `internal/gamestate/processor.go` — `pendingStatChange` struct, append site, `flushPendingStatChanges`

## Complexity

Medium — requires understanding of when `blockSource`/`blockCardID` are set relative
to when TAG_CHANGE events arrive.

## Verification

- Integration test: confirm `Modifications[].CardID` and `Source` are non-empty for
  known board-wide buffs in the sample log.
- Check that `Category` is correctly resolved via `categoryByEnchantmentCardID`.
