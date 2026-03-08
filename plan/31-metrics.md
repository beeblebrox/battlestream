# 31 — [IMPROVEMENT] No metrics/observability

**Priority:** LOW
**Area:** `internal/api/`, daemon

## Problem

No Prometheus or similar metrics are exported. There is no way to observe the daemon's
operational health, throughput, or error rates without reading log files. Useful counters
are absent for:
- Lines parsed per second
- Events emitted per type
- Games completed / in-progress
- Buff sources updated
- Channel full / dropped events (if plan 07 non-blocking send is implemented)

## Fix

### Step 1: Add a metrics endpoint

Add `GET /metrics` to the REST server that returns Prometheus-format text.

### Step 2: Define key counters

```go
var (
    linesParsed     = prometheus.NewCounter(...)
    eventsEmitted   = prometheus.NewCounterVec(..., []string{"type"})
    gamesCompleted  = prometheus.NewCounter(...)
    buffSourcesSeen = prometheus.NewCounter(...)
    droppedEvents   = prometheus.NewCounter(...)
)
```

### Step 3: Instrument key paths

- Parser `Feed()`: increment `linesParsed` on each call
- Parser output: increment `eventsEmitted` with event type label
- Processor GameEnd handler: increment `gamesCompleted`
- Channel full drop (plan 07): increment `droppedEvents`

### Step 4: Add dependency

```
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

## Files to change

- `go.mod` — add prometheus/client_golang
- `internal/api/rest/` — add `/metrics` route
- `internal/parser/parser.go` — increment `linesParsed`, `eventsEmitted`
- `internal/gamestate/processor.go` — increment `gamesCompleted`, `buffSourcesSeen`

## Complexity

Medium — new dependency, instrumentation throughout the codebase.

## Verification

- `curl http://127.0.0.1:8080/metrics` returns valid Prometheus text format.
- After feeding a log, `eventsEmitted` counter has non-zero values for known event types.
