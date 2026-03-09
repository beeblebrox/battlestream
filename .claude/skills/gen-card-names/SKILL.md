---
name: gen-card-names
description: gen-card-names skill
disable-model-invocation: true
---

# gen-card-names

Generate `internal/gamestate/cardnames.go` from the live HearthstoneJSON card database.

## When to use

- After a new Battlegrounds patch adds new heroes or minions
- When card names appear as raw card IDs in the TUI
- To rebuild the mapping after the cache expires (7 days)

## Usage

```
/gen-card-names [--force] [--dry-run]
```

| Flag | Description |
|------|-------------|
| `--force` | Bypass the 7-day cache and re-fetch from HearthstoneJSON |
| `--dry-run` | Print the generated Go file without writing it |

## What it generates

`internal/gamestate/cardnames.go` — a Go source file containing:
- `var cardNames map[string]string` — all BG hero and minion card IDs → display names
- `func CardName(cardID string) string` — lookup function (returns cardID if not found)

## Instructions

Run the script:
```bash
.claude/skills/gen-card-names/gen_card_names.py [--force] [--dry-run]
```

Then rebuild:
```bash
go build ./...
```

Then run tests to confirm nothing broke:
```bash
go test ./internal/gamestate/ -timeout 60s
```
