# 27 — [IMPROVEMENT] REST deep-copies full state on every poll

**Priority:** LOW
**Area:** `internal/gamestate/machine.go` — `State()`, `internal/api/rest/`

## Problem

`machine.State()` deep-copies all slices (board, enchantments, mods, buff sources) on
every call. For polling clients or SSE subscribers, this creates GC pressure proportional
to board complexity. On a full board (7 minions, each with multiple enchantments), every
poll allocates dozens of slice copies.

## Fix

### Option A: Generation counter / dirty flag

Add a `generation uint64` counter to the machine, incremented on every state mutation.
Expose it as `machine.Generation()`. Polling clients send their last-seen generation
with each request; if unchanged, the server returns 304 Not Modified.

```go
// In REST handler:
clientGen := r.Header.Get("If-None-Match")
if clientGen == strconv.FormatUint(machine.Generation(), 10) {
    w.WriteHeader(http.StatusNotModified)
    return
}
```

### Option B: Cached serialized snapshot

After each state mutation, serialize to JSON once and cache the bytes + generation.
`State()` for REST returns the cached bytes directly, avoiding repeated allocation.

```go
type Machine struct {
    // ...
    cachedJSON []byte
    generation uint64
}
```

On mutation: `m.cachedJSON = nil; m.generation++`
On read: if `m.cachedJSON != nil { return m.cachedJSON }; marshal and cache.`

Option B is simpler and eliminates allocation for all polling clients simultaneously.

### Option C: Reduce polling frequency with SSE

Encourage clients to use SSE/WebSocket rather than polling. SSE pushes only on change,
eliminating wasteful no-change polls. See also plan 29.

## Files to change

- `internal/gamestate/machine.go` — add generation counter or cached JSON
- `internal/api/rest/` — add ETag / 304 support

## Complexity

Medium. Option B is the simplest implementation.

## Verification

- Benchmark `machine.State()` before and after; confirm allocation count drops.
- REST client receiving 304 on unchanged state and full response on change.
