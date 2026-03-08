# 07 — [RISK] Parser→Processor channel undocumented/unbuffered

**Priority:** HIGH
**Status:** DONE
**Area:** `internal/parser/parser.go`, daemon startup, `internal/watcher/`

## Problem

The `chan GameEvent` between parser and processor has no documented buffer size contract.
In the daemon, if the processor blocks (e.g., waiting on a mutex or a slow store write),
the parser blocks on `p.out <- e`, which blocks the watcher goroutine, which stops reading
from `nxadm/tail`, which can cause the tail's internal buffer to fill and lines to be dropped.

Under log replay (`battlestream reparse`), this risk is higher: large logs feed events
at full CPU speed with no back-pressure relief.

## Fix

### Step 1: Document the contract

Add a comment at the channel creation site (daemon startup and/or watcher) specifying
the intended buffer size and the rationale.

### Step 2: Buffer the channel

A buffer of 256–1024 events is sufficient to absorb short processor stalls without
blocking the parser. Tune based on typical events-per-second during heavy combat phases.

```go
// In daemon / watcher setup:
eventCh := make(chan GameEvent, 512) // buffered to absorb processor stalls
```

### Step 3: Add a drop counter (optional but recommended)

If the channel is ever full (buffer exhausted), log a warning rather than blocking
indefinitely. This requires a non-blocking send with a select:

```go
select {
case p.out <- e:
default:
    slog.Warn("event channel full, dropping event", "type", e.Type)
    // increment a metrics counter (see plan 31)
}
```

Note: dropping events is less harmful than blocking the tail reader. The processor
is designed to be stateful and tolerant of missing intermediate events.

## Files to change

- Daemon startup code (`cmd/battlestream/main.go` or daemon subcommand)
- `internal/parser/parser.go` — optionally switch to non-blocking send with drop log
- `internal/watcher/` — wherever the channel is created

## Complexity

Low — buffer size change + comment. Non-blocking send is optional but adds safety.

## Resolution

Fixed: All three steps implemented.
- Channel buffered at 512 in `cmd/battlestream/main.go` (both daemon and reparse paths).
- Parser `emit()` uses non-blocking `select` with a `default` case that logs a warning
  and drops the event rather than blocking the tail reader.
