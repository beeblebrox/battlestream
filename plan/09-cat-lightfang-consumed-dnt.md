# 09 — [RISK] `CatLightfang` and `CatConsumed` have no Dnt handlers

**Priority:** HIGH
**Status:** DONE
**Area:** `internal/gamestate/processor.go` — `handleDntTagChange`, `categories.go`

## Problem

`CatLightfang` and `CatConsumed` are defined in `categories.go` and appear in
`categoryByEnchantmentCardID`, so per-minion enchantments are added to `machine.AddEnchantment`
correctly. However, there is no corresponding `case` in `handleDntTagChange`.

If these categories use cumulative Dnt counters (like other buff categories), their
`BuffSource` counters in the machine will never be updated. The TUI BUFF SOURCES panel
will show `0/0` for them, or they will be absent entirely.

## Investigation required

Before implementing, determine whether Lightfang and Consumed actually use Dnt counters:

1. Check HDT reference `BgCounters/` — is there a counter class for either?
2. Check `reference/HearthDb/` — are there `BACON_*` GameTag entries for them?
3. Check the sample log for any `TAG_CHANGE` with entity matching Lightfang/Consumed
   enchantments and a `tag=SD` or `tag=BACON_*` pattern.

**If they use Dnt counters:** add the missing `case` in `handleDntTagChange` analogous
to the other categories.

**If they are purely per-minion (no Dnt counter):** document this explicitly in
`categories.go` and in `PARSER_SPEC.md` — add a comment saying "no Dnt handler needed"
so future contributors don't add one unnecessarily.

## Fix (if Dnt counters exist)

```go
case CatLightfang:
    // tag SD encodes total Lightfang buff stacks
    p.machine.SetBuffSource(CatLightfang, BuffSource{ATK: val, HP: val})
case CatConsumed:
    p.machine.SetBuffSource(CatConsumed, BuffSource{ATK: val, HP: 0})
```

Exact field mapping depends on what the HDT counter class reveals.

## Files to change

- `internal/gamestate/processor.go` — `handleDntTagChange` (add cases if needed)
- `internal/gamestate/categories.go` — add explanatory comment if no Dnt handler needed
- `docs/PARSER_SPEC.md` — document the finding

## Complexity

Low-medium — requires research before coding. The buff-scout agent workflow is the right
tool for the research phase.

## Resolution

Investigation complete: CatLightfang and CatConsumed have no player-level Dnt counters.
HDT has no LightfangCounter.cs or ConsumedCounter.cs. They are purely per-minion
enchantments tracked via `handleEnchantmentEntity`. Explicit comment added in
`handleDntTagChange` documenting this finding.
