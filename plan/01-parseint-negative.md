# 01 — [BUG] `parseInt` silently accepts negative numbers as positive

**Priority:** CRITICAL
**Area:** `internal/gamestate/processor.go` (or wherever `parseInt` is defined)

## Problem

```go
func parseInt(s string) int {
    n := 0
    for _, c := range s {
        if c >= '0' && c <= '9' { n = n*10 + int(c-'0') }
    }
    return n
}
```

The `-` character is silently skipped. `parseInt("-5")` returns `5`. Any Hearthstone tag
that legitimately carries a negative value (debuff, reduced ATK, negative delta) will be
read as a positive number, causing silently wrong state — buff deltas inverted, health
values wrong, etc.

Known legitimate negatives in HS logs:
- ATK debuffs (e.g. `-2`)
- HEALTH debuffs
- Hypothetically any future tag using signed values

## Impact

Silent data corruption. No error is raised; the wrong value propagates into `Machine` state,
affecting board stats, buff attribution, and anything downstream (REST, gRPC, TUI, store).

## Fix

Replace `parseInt` with `strconv.Atoi` (or a thin wrapper that logs on error and returns 0).

```go
import "strconv"

func parseInt(s string) int {
    n, err := strconv.Atoi(strings.TrimSpace(s))
    if err != nil {
        return 0
    }
    return n
}
```

If zero-on-error is undesirable for some callers, introduce a `parseIntOr(s string, def int) int`
variant and audit all call sites to choose the appropriate default.

## Files to change

- `internal/gamestate/processor.go` — replace `parseInt` definition and audit all call sites
- `internal/parser/parser.go` — if `parseInt` is also used here, apply same fix

## Complexity

Low — mechanical replacement. Requires a grep for all `parseInt(` call sites to verify
none depend on the sign-stripping behaviour.

## Verification

Add a unit test:
```go
assertEqual(parseInt("-5"), -5)
assertEqual(parseInt("0"), 0)
assertEqual(parseInt("42"), 42)
assertEqual(parseInt("bad"), 0)
```
