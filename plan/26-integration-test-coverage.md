# 26 — [IMPROVEMENT] Integration test coverage limited to one log

**Priority:** LOW
**Area:** `internal/gamestate/`, `testdata/`

## Problem

`testdata/power_log_game.txt` (92K lines) covers one specific game. The following
edge cases are not covered by automated tests:

- **Nomi All** (every minion gets buffed by Nomi each turn — high buff volume)
- **Timewarped** mechanics (altered turn structure, extra phases)
- **Late-game high-health** scenarios (large numbers, potential parseInt issues)
- **Games ending during combat** (game-over detected while in PhaseCombat)
- **Midnight log** (timestamps wrapping midnight)
- **Multi-game session** (two games back-to-back in one log file)
- **Truncated log** (parser resilience to mid-block EOF)

## Fix

### Step 1: Collect additional log samples

Capture anonymized Power.log excerpts for each edge case above. Even short synthetic
logs (hand-crafted sequences) are valuable for unit testing specific scenarios.

### Step 2: Parameterize the integration test

Restructure the existing integration test to be table-driven:
```go
var integrationCases = []struct {
    log      string
    expected GameSnapshot
}{
    {"testdata/power_log_game.txt", expectedGame1},
    {"testdata/nomi_game.txt", expectedNomiGame},
    // ...
}
```

### Step 3: Add targeted unit tests for each edge case

Rather than requiring full log files, write unit tests that feed synthetic event
sequences for each scenario:

- `TestParserMidnightWrap` — feed events spanning midnight
- `TestProcessorGameEndDuringCombat` — feed game-end event during PhaseCombat
- `TestProcessorMultiGame` — feed two game sequences back-to-back
- `TestParserTruncatedBlock` — feed partial BLOCK_START without BLOCK_END

## Files to change

- `internal/gamestate/processor_test.go` — new test cases
- `internal/parser/parser_test.go` — new parser edge case tests
- `testdata/` — new log samples (anonymized)

## Complexity

Medium — test authoring. No production code changes.

## Verification

All new tests pass. Existing integration test continues to pass unchanged.
