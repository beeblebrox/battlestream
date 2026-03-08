# 14 — [IMPROVEMENT] No panic recovery in `Feed()` — daemon crashes on unexpected log format

**Priority:** MEDIUM
**Area:** `internal/parser/parser.go` — `Feed()`

## Problem

If a regex match produces an unexpected capture group layout (e.g., due to a Blizzard
log format change), the parser will panic with an index-out-of-range. There is no
`recover()` wrapper. A single malformed line will crash the daemon goroutine, stopping
all log processing for the session.

## Fix

Wrap the body of `Feed()` in a deferred recover that logs the offending line and
continues processing:

```go
func (p *Parser) Feed(line string) {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("parser panic", "err", r, "line", line)
        }
    }()
    // existing logic ...
}
```

This makes the daemon resilient to unexpected log format changes — bad lines are logged
and skipped rather than crashing the process.

### Additional hardening

For regex capture-group accesses, add explicit length checks before indexing:

```go
m := reTagChange.FindStringSubmatch(line)
if len(m) < 5 {
    slog.Warn("reTagChange: unexpected match length", "line", line, "len", len(m))
    return
}
```

This prevents the panic in the first place and provides a more informative error message
than a generic index-out-of-range.

## Files to change

- `internal/parser/parser.go` — add deferred recover to `Feed()`; add length guards
  before capture-group accesses

## Complexity

Low — defensive additions. No logic changes.

## Verification

- Unit test: `Feed()` with a line that would cause a capture group panic (e.g., a
  `TAG_CHANGE` line with a regex match but fewer groups than expected) should log an
  error and not panic.
- Daemon should survive feeding a completely garbage line without crashing.
