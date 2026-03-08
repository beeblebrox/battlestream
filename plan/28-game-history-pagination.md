# 28 — [IMPROVEMENT] No game history endpoint pagination

**Priority:** LOW
**Area:** `internal/api/rest/`, `internal/store/`

## Problem

`GET /v1/stats/games` returns all stored games. As the store grows over multiple sessions
(months of daily play), this response becomes large. There is no cursor or page parameter,
so clients must always download the full history.

## Fix

### Step 1: Add cursor-based pagination to the store layer

Extend `store.ListGames()` to accept a cursor (last-seen game ID) and a limit:

```go
func (s *Store) ListGames(afterID string, limit int) ([]GameSummary, string, error)
// returns: games, nextCursor, error
```

BadgerDB iteration supports seek-to-key, making cursor pagination efficient.

### Step 2: Add query parameters to the REST endpoint

```
GET /v1/stats/games?limit=20&after=<cursor>
```

Response includes a `next_cursor` field clients use for the next page:

```json
{
  "games": [...],
  "next_cursor": "game-1234567890",
  "total": 142
}
```

### Step 3: gRPC equivalent

Add `page_size` and `page_token` fields to `ListGamesRequest` in the proto, and return
`next_page_token` in `ListGamesResponse`.

## Files to change

- `internal/store/` — cursor-based `ListGames`
- `internal/api/rest/` — add `limit` and `after` query params
- `internal/api/grpc/` — add pagination fields to handler
- Proto file — add pagination fields to `ListGamesRequest`/`ListGamesResponse`

## Complexity

Medium — store layer change + API surface change. Proto change requires codegen.

## Verification

- Query `/v1/stats/games?limit=5` with >5 games stored; assert exactly 5 returned
  with a valid `next_cursor`.
- Follow cursor and retrieve second page; assert no overlap or gaps.
