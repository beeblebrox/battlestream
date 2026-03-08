# 17 — [RISK] Enchantment CardID table manually curated — new mechanics missed silently

**Priority:** MEDIUM
**Area:** `internal/gamestate/categories.go`

## Problem

`categories.go` contains a hardcoded map from CardID strings to buff categories. Every
new BG season or patch that introduces new buff mechanics requires a manual update.
There is no mechanism to detect missing entries at runtime — an unseen CardID simply
produces no category match and is silently treated as an unknown buff.

## Mitigation and Fix

### Step 1: Log unknown enchantment CardIDs at runtime

In the processor, whenever `categoryByEnchantmentCardID[cardID]` returns an empty/zero
value for a cardID that is actively applied to a BG minion, log a warning:

```go
if cat, ok := categoryByEnchantmentCardID[cardID]; !ok || cat == 0 {
    slog.Warn("unknown enchantment CardID — categories.go may need update",
        "cardID", cardID, "entity", entityName)
}
```

This surfaces new mechanics in daemon logs immediately after a patch.

### Step 2: Automated audit tooling

Extend the buff-scout agent workflow (`.claude/agents/buff-scout.md`) to:
1. Scan the HDT reference `BgCounters/` directory for new counter classes after each patch.
2. Cross-reference against the current `categoryByEnchantmentCardID` map.
3. Report any HDT counter CardIDs not present in the map.

Optionally, write a Go test that loads the HDT reference CardID list and asserts every
HDT-known BG enchantment has an entry in `categoryByEnchantmentCardID`.

### Step 3: Document the update procedure

Add a comment at the top of `categories.go`:

```
// MAINTENANCE: After each BG patch, run the buff-scout workflow to check for
// new enchantment CardIDs. Add any new entries here and update PARSER_SPEC.md.
```

## Files to change

- `internal/gamestate/processor.go` — add unknown CardID warning
- `internal/gamestate/categories.go` — add maintenance comment
- `.claude/agents/buff-scout.md` — extend with cross-reference step
- (Optional) `internal/gamestate/categories_test.go` — HDT cross-reference test

## Complexity

Low (warning log) + medium (test automation).

## Verification

- Feed a crafted event with an unknown CardID; assert warning appears in daemon log.
- Buff-scout agent produces a diff report showing any gap between HDT and categories.go.
