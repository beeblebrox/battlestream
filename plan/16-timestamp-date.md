# 16 — [BUG] Timestamp uses today's date — midnight wrap and wrong reparse dates

**Priority:** MEDIUM
**Status:** DONE
**Area:** `internal/parser/parser.go` — `extractTimestamp`

**Resolution:** `extractTimestamp` now uses `refDate` (settable via `SetReferenceDate`)
instead of `time.Now()`. Midnight wrap detection advances refDate when timestamps jump
backwards by >12 hours. Default refDate is today for live tailing; reparse can set it
to the log file's modification time.

## Problem

`extractTimestamp` combines today's date with the parsed `HH:MM:SS` from the log line.
Two consequences:

1. **Midnight wrap:** A game that crosses midnight produces events with timestamps that
   jump backwards (e.g., 23:59:xx followed by 00:01:xx using today's date will show
   the second event as ~24 hours earlier).

2. **Wrong reparse dates:** Reparsing an old log assigns today's date to every event,
   regardless of when the log was actually written. This makes historical timestamps
   meaningless in the store.

## Fix

### Option A: Use log file mtime (simplest)

Pass the log file's modification time (or creation time) to the parser as a reference
date. Use that date for all events unless a midnight wrap is detected.

```go
func (p *Parser) SetReferenceDate(t time.Time) {
    p.refDate = t.Truncate(24 * time.Hour)
}

func (p *Parser) extractTimestamp(hh, mm, ss int) time.Time {
    ts := time.Date(p.refDate.Year(), p.refDate.Month(), p.refDate.Day(),
        hh, mm, ss, 0, time.Local)
    // detect midnight wrap
    if p.lastTS != (time.Time{}) && ts.Before(p.lastTS) {
        p.refDate = p.refDate.Add(24 * time.Hour)
        ts = ts.Add(24 * time.Hour)
    }
    p.lastTS = ts
    return ts
}
```

### Option B: Session-start date

When the first `CREATE_GAME` line of a session is seen, record that as the session date.
Use it for all subsequent events. Midnight wrap detection same as Option A.

Option A is simpler for the reparse case because the file mtime is available at open time.

## Files to change

- `internal/parser/parser.go` — `extractTimestamp`, add `refDate` and `lastTS` fields
- `internal/watcher/` or daemon startup — pass file mtime to parser via `SetReferenceDate`
- `cmd/battlestream/` reparse command — pass log file mtime

## Complexity

Low-medium. Midnight wrap detection is a small addition; mtime threading is mechanical.

## Verification

- Unit test: feed events spanning midnight with a fixed reference date; assert no
  backwards timestamps.
- Integration test: timestamps on reparsed log should have the date of the test file's
  mtime, not today.
