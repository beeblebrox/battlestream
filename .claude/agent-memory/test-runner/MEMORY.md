# Test Runner Agent Memory

## Golden Files
- `internal/debugtui/testdata/golden/` holds TUI screenshot golden files
- Regenerate with: `go test ./internal/debugtui/ -run TestDump_Golden -update-golden -count=1 -timeout 60s`
- Regenerate whenever `debugtui` rendering logic or `jumpToTurn` behavior changes
- Also regenerate when `AvailableTribes` ordering changes (order is log-event order from `AddAvailableTribe`)
- The `-update-golden` flag is defined via `flag.Bool` in replay_test.go

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
| internal/debugtui | ~38s | 3 golden tests × LoadReplay (multiple full replays) |
| internal/gamestate | ~88s | all log-2026 tests share 1 parse; 592K line file |

See `patterns.md` for more architectural details.

## GameID Scheme (changed — timestamp-based)
- Production code generates `game-<unixmilli>` when EventGameStart has a non-zero Timestamp
- Falls back to sequential `game-<n>` only when Timestamp is zero
- Tests using sequential IDs must pass `Timestamp: time.Time{}` (zero value)
- Tests asserting on the ID format should use `strings.HasPrefix(id, "game-")` or compute the expected value via `fmt.Sprintf("game-%d", ts.UnixMilli())`
- `TestProcessorGameStartTimestampID` covers the timestamp path; `TestProcessorGameStartIncrementsID` covers sequential fallback

## Win/Loss Streak Detection Bug (fixed 2026-03-07)
- BG GameState log contains TWO types of PROPOSED_ATTACKER events:
  1. Real-time combat block: fires during actual combat, reliable for win/loss
  2. Post-combat simulation replay block: fires after combat at a later timestamp, UNRELIABLE
- The simulation replay may show `PROPOSED_ATTACKER=localHeroID` even when the local player LOST
- Fix: added `localHeroTookDamage bool` to Processor; set when local hero ARMOR or HEALTH decreases
- At TURN boundary: `localHeroTookDamage=true` → LOSS (overrides PROPOSED_ATTACKER signal)
- PROPOSED_ATTACKER is still used as secondary signal when hero took no damage
- Relevant test: `TestGameLog2026_03_07_WinLossStreak` expects WinStreak=2 (rounds 14+15), LossStreak=0

## GoldNextTurn Display Format
- Production format (processor.go `updateGoldNextTurnCounter`): `"%d (+%d if win)"` when bonus > 0
- Example: 2 sure + 3 overconfidence bonus → `"2 (+3 if win)"` (NOT `"2 (5)"`)
- Tests `TestCounterGoldNextTurnWithOverconfidence` and `TestCounterGoldNextTurnMultipleOverconfidence` use this format
- The bonus is conditional on winning combat, hence the `"if win"` phrasing — do NOT revert to total-sum format

## Naga Synergy Counter Pattern (tag 3809)
- `HasNagaSynergyMinion(board)` gates whether tag=3809 emits or removes `CatNagaSpells`
- Unit tests for tag=3809 must call `setupNagaMinion(p)` before firing the tag event
- `setupNagaMinion` adds entity 500 (`BG31_924` Thaumaturgist, MINION, ZONE=PLAY, CONTROLLER=7)
- Integration test `TestGameLog2026_03_07_NagaSpellsFinal` asserts counter is ABSENT (Spectre Teron game has no synergy minions)
- Golden files regenerate WITHOUT "Naga Spells" / "Spells Played" row when synergy minion absent from board
- `CategoryDisplayName[CatNagaSpells]` = "Spells Played" (NOT "Naga Spells" — name changed)
