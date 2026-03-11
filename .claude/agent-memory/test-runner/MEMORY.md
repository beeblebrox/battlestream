# Test Runner Agent Memory

## Golden Files
- `internal/debugtui/testdata/golden/` holds TUI screenshot golden files
- Regenerate with: `go test battlestream.fixates.io/internal/debugtui -run TestDump_Golden -update-golden -count=1 -timeout 60s`
- NOTE: `./internal/debugtui/` relative path may fail if shell cwd is reset; prefer the import path form
- Regenerate whenever `debugtui` rendering logic, `jumpToTurn` behavior, or buff source category names/values change
- Also regenerate when `AvailableTribes` ordering changes (order is log-event order from `AddAvailableTribe`)
- The `-update-golden` flag is defined via `flag.Bool` in replay_test.go
- CRITICAL: Also regenerate when `Step.Turn` assignment in `replay.go` changes — it affects `jumpToTurn` landing position
- Golden test cases use turn=8, turn=10, turn=0 (log fixture starts at turn 8 — mid-game reconnect log)
- If PLAYER_DEF now sets Turn early (from TURN tag in reconnect), jumpToTurn(1) or jumpToTurn(5) lands at step 2 (Turn 8), NOT step 3889 (Turn 9)

## Large Log Fixtures
- `internal/gamestate/testdata/power_log_2026_03_07.txt` is 593K lines
- `internal/gamestate/testdata/power_log_game.txt` is 92K lines (original integration fixture)
- Under `-race` detector, parsing 593K lines takes ~5-6s; 8 tests x 5s = ~40s total
- **Do NOT re-parse large log files per-test.** Use `sync.Once` shared state.
- Pattern lives in `log2026_helper_test.go` (`sharedLog2026State(t)` function)

## Race Detector Timing
- `-race` adds ~10x overhead to CPU-bound work (regexp matching, log parsing)
- A 120s global timeout with multiple large-log tests WILL fail under `-race`
- Safe runtime: `go test ./... -race -timeout 120s` passes if log parsing is shared
- `internal/debugtui` tests take ~99s under `-race` (multiple full replays with LoadReplay)
- `internal/gamestate` tests take ~44s under `-race` with `sync.Once` sharing

## Test File Structure
- `internal/gamestate/gamestate_test.go` — `package gamestate` (white-box unit tests)
- `internal/gamestate/game_log_2026_03_07_test.go` — `package gamestate_test` (integration)
- `internal/gamestate/log2026_helper_test.go` — `package gamestate_test` (shared parse helper)
- `internal/debugtui/replay_test.go` — `package debugtui` (white-box)

## No testify
- Project uses standard `testing` package assertions only (no testify in go.mod)

## Running Tests
- Standard: `go test ./... -race -timeout 300s -count=1` (120s is too tight; debugtui takes ~99s)
- Skip race (faster): `go test ./... -timeout 60s -count=1`
- Single package: `go test ./internal/gamestate/ -race -timeout 120s -count=1 -v`

## Known Slow Packages Under Race
| Package | Race time | Reason |
|---------|-----------|--------|
| internal/debugtui | ~41s | 3 golden tests × LoadReplay (multiple full replays) |
| internal/gamestate | ~108s | all log-2026 tests share 1 parse; 592K line file; Duos tests added |

See `patterns.md` for more architectural details.

## TUI Layout Bug Patterns

### internal/tui — vpContentW calculation
- `styleBorder` has `Padding(0, 1)` → inner content width = `colW - 4` (2 border + 2 padding)
- Viewport + scrollbar must fit within inner area: `vpContentW + 1 = colW - 4` → `vpContentW = colW - 5`
- Previous code had `vpContentW = colW - 1` which caused 4-char overflow at narrow widths (80, 100)
- Overflow causes lipgloss to wrap content lines, inflating panel height above budget
- Fixed in `internal/tui/tui.go` View() function
- Symptom: `TestView_MultipleWidths` fails at narrow widths with output height > terminal height

### internal/debugtui — player panel height vs viewport budget
- `renderPlayerPanel` renders all content without height capping; can grow to 9+ lines
- Budget formula assumed row2 = `maxContentH + 3`, but playerPanel is not bounded by `maxContentH`
- When `playerPanelH > maxContentH + 3`, rawH gets clamped up from negative, adding extra lines
- Fix: compute playerPanel first, derive `boardVPH = playerPanelH - 3` to match it, recalculate row3 budget
- Symptom: `TestDump_FitsWithinHeight/turn=0/w=120/h=30` fails with 31 lines instead of 30
- Reproduces when late-game player panel has many fields (name+health+armor+triples+hero+last+win = 7 content + 2 border = 9 lines)
- Fix in `viewStep()` in `internal/debugtui/model.go`; requires golden file regeneration after

## GameID Scheme (changed — timestamp-based)
- Production code generates `game-<unixmilli>` when EventGameStart has a non-zero Timestamp
- Falls back to sequential `game-<n>` only when Timestamp is zero
- Tests using sequential IDs must pass `Timestamp: time.Time{}` (zero value)
- Tests asserting on the ID format should use `strings.HasPrefix(id, "game-")` or compute the expected value via `fmt.Sprintf("game-%d", ts.UnixMilli())`
- `TestProcessorGameStartTimestampID` covers the timestamp path; `TestProcessorGameStartIncrementsID` covers sequential fallback

## Win/Loss Streak Detection (current implementation — 2026-03-10)
- Processor uses `pendingHeroAttackerID` + `localCombatResult` (replaces old `lastCombatHeroAttackerID`)
- PROPOSED_ATTACKER on GameEntity → buffers `pendingHeroAttackerID` (only if it's a hero entity)
- PROPOSED_DEFENDER on GameEntity → if both attacker and defender are heroes, resolve local hero role:
  - local hero is attacker → `localCombatResult = 1` (win)
  - local hero is defender → `localCombatResult = -1` (loss)
  - neither is local hero → ignore (partner's combat in Duos)
- At TURN boundary: `localCombatResult > 0` → RecordRoundWin; `< 0` → RecordRoundLoss; reset to 0
- At EventGameEnd: record final round from `localCombatResult` before calling GameEnd (last combat has no TURN boundary)
- Relevant test: `TestGameLog2026_03_07_WinLossStreak` expects WinStreak=12, LossStreak=0

## Final-Round Streak Fix (2026-03-10)
- The TURN-based streak update never fires after the last combat (game ends before TURN=N+1)
- Fix: at EventGameEnd handler, flush `localCombatResult` → RecordRoundWin/Loss before calling GameEnd
- After fix: `TestGameLog2026_03_07_WinLossStreak` expects WinStreak=12 (was 11 pre-fix)
- After fix: `last-turn.txt` golden file shows `streak: 4` (was `streak: 3` pre-fix)
- Whenever this fix or its revert changes, regenerate golden files AND update the WinStreak assertion

## GoldNextTurn Display Format
- Production format (processor.go `updateGoldNextTurnCounter`): `"%d (+%d if win)"` when bonus > 0
- Example: 2 sure + 3 overconfidence bonus → `"2 (+3 if win)"` (NOT `"2 (5)"`)
- Tests `TestCounterGoldNextTurnWithOverconfidence` and `TestCounterGoldNextTurnMultipleOverconfidence` use this format
- The bonus is conditional on winning combat, hence the `"if win"` phrasing — do NOT revert to total-sum format

## GameStart Mutex Bug Pattern (CRITICAL)
- `Machine.GameStart` previously used `*m = Machine{}` to reset state — THIS PANICS
- `*m = Machine{}` zeroes the embedded `sync.RWMutex` while it is locked → `defer m.mu.Unlock()` panics: "sync: Unlock of unlocked RWMutex"
- Fix: reset each field explicitly (state, gameEntityTurn, boardSnapshot, goldTotal, goldUsed, partnerGoldTotal, partnerGoldUsed) WITHOUT touching the mutex
- The `deepCopyBoard` helper and `UpdateBoardSnapshot` method were added in this same change (board snapshot deep copy)
- Symptom: panic on first test that calls `GameStart` (e.g. `TestDebugTurn8Board`)
- Fixed in `internal/gamestate/state.go` `GameStart()` method

## Known Slow Packages Under Race (Updated Timing)
- `internal/gamestate` now takes ~96s under `-race` (two large log parses share sync.Once each)
- `internal/debugtui` takes ~36s under `-race`
- Total suite time under `-race -timeout 300s`: ~140s (well within 300s)

## Naga Synergy Counter Pattern (tag 3809)
- `HasNagaSynergyMinion(board)` gates whether tag=3809 emits or removes `CatNagaSpells`
- Unit tests for tag=3809 must call `setupNagaMinion(p)` before firing the tag event
- `setupNagaMinion` adds entity 500 (`BG31_924` Thaumaturgist, MINION, ZONE=PLAY, CONTROLLER=7)
- Integration test `TestGameLog2026_03_07_NagaSpellsFinal` asserts counter is ABSENT (Spectre Teron game has no synergy minions)
- Golden files regenerate WITHOUT "Naga Spells" / "Spells Played" row when synergy minion absent from board
- `CategoryDisplayName[CatNagaSpells]` = "Spells Played" (NOT "Naga Spells" — name changed)
