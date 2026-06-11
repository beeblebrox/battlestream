// Package debugtui implements a step-by-step Power.log replay TUI for debugging.
package debugtui

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/gamestate/builder"
	"battlestream.fixates.io/internal/gamestate/card"
	"battlestream.fixates.io/internal/parser"
	"battlestream.fixates.io/internal/store"
)

// playerLogPowerPrefix is the category tag prepended to Power lines in Player.log.
const playerLogPowerPrefix = "[Power] "

// maxRawLinesPerStep caps how many raw log lines a single Step retains.
// The RAW LOG panel is a scrollable viewport, so a couple hundred lines is
// far more than is ever read for one step; without a cap, steps that absorb
// many collapsed no-change events would grow without bound. When truncated,
// Step.RawLinesDropped records how many leading lines were discarded and the
// RAW LOG panel header notes the truncation.
const maxRawLinesPerStep = 200

// Step represents a single parsed event with its associated state snapshot.
type Step struct {
	Index           int
	Event           parser.GameEvent
	RawLines        []string              // log lines that produced this event (capped at maxRawLinesPerStep)
	RawLinesDropped int                   // count of leading raw lines truncated by the cap
	State           gamestate.BGGameState // snapshot AFTER processing this event
	Turn            int                   // BG turn number at this point
}

// appendRawLines appends lines to dst, enforcing maxRawLinesPerStep by
// discarding the oldest lines. It returns the new slice and the updated
// dropped-line count. The truncated slice is reallocated so the original
// oversized backing array can be garbage collected.
func appendRawLines(dst []string, dropped int, lines []string) ([]string, int) {
	dst = append(dst, lines...)
	if over := len(dst) - maxRawLinesPerStep; over > 0 {
		dst = append([]string(nil), dst[over:]...)
		dropped += over
	}
	return dst, dropped
}

// GameSummary describes a single game found during parsing.
type GameSummary struct {
	Index      int       // game number (0-based)
	PlayerName string    // local player name
	HeroCardID string    // hero card ID
	Placement  int       // final placement (0 if incomplete)
	MaxTurn    int       // highest turn reached
	TavernTier int       // final tavern tier
	Phase      gamestate.GamePhase
	StartTime  time.Time
	SourceFile string    // which log file this game came from
	StepStart  int       // first step index in the global steps slice
	StepEnd    int       // one past last step index
	IsDuos     bool      // whether this is a Duos game
}

// Replay holds the complete parsed result: all steps and game summaries.
type Replay struct {
	Steps []Step
	Games []GameSummary
	// EventCount is the total number of parsed events, including events that
	// produced no visible state change and were collapsed into the previous
	// Step (so len(Steps) <= EventCount).
	EventCount int
}

// LoadReplay parses a single Power.log file into steps grouped by game.
func LoadReplay(path string) (*Replay, error) {
	return LoadAllGames([]string{path})
}

// LoadAllGames parses multiple Power.log files into a unified replay (no progress tracking).
func LoadAllGames(paths []string) (*Replay, error) {
	return LoadAllGamesWithProgress(paths, nil)
}

// LoadAllGamesWithProgress parses multiple Power.log files, updating progress atomics.
func LoadAllGamesWithProgress(paths []string, prog *loadProgress) (*Replay, error) {
	ch := make(chan parser.GameEvent, 256)
	p := parser.New(ch)
	machine := gamestate.New()
	proc := gamestate.NewProcessor(machine)
	catalog := card.NewFuncCatalog(gamestate.CardName)
	bldr := builder.New(catalog)
	disp := gamestate.NewActionDispatcher(
		proc.AsRecruitVisitor(),
		proc.AsCombatVisitor(),
		proc.AsTransitionVisitor(),
		proc.Touch,
	)

	var steps []Step
	var games []GameSummary
	var accum []string
	var currentFile string
	eventCount := 0

	// Track current game boundary.
	gameStepStart := -1

	// foldAccum merges pending raw lines into the most recent step so the
	// RAW LOG panel still shows them, then clears the accumulator. Used for
	// events that produced no visible state change (collapsed steps) and for
	// trailing lines at file boundaries. If no step exists yet, the lines are
	// dropped.
	foldAccum := func() {
		if len(accum) > 0 && len(steps) > 0 {
			prev := &steps[len(steps)-1]
			prev.RawLines, prev.RawLinesDropped = appendRawLines(prev.RawLines, prev.RawLinesDropped, accum)
		}
		accum = nil
	}

	drain := func() {
		for {
			select {
			case evt := <-ch:
				if a := bldr.Build(evt); a != nil {
					_ = disp.Dispatch(a)
				} else {
					proc.Handle(evt)
				}
				eventCount++
				snap := machine.State()

				// Snapshot dedup (audit H7): most events change nothing the
				// snapshot can see, yet each deep-copied BGGameState retained
				// per event made memory O(events × state-size). Only append a
				// new Step when the state actually changed; otherwise fold the
				// event's raw lines into the previous step. Game start/end
				// events always get a step because game boundary bookkeeping
				// (gameStepStart/StepEnd) points at them.
				forced := evt.Type == parser.EventGameStart || evt.Type == parser.EventGameEnd
				if !forced && len(steps) > 0 && reflect.DeepEqual(steps[len(steps)-1].State, snap) {
					foldAccum()
					continue
				}

				step := Step{
					Index: len(steps),
					Event: evt,
					State: snap,
					Turn:  snap.Turn,
				}
				step.RawLines, step.RawLinesDropped = appendRawLines(nil, 0, accum)
				steps = append(steps, step)
				accum = nil

				switch evt.Type {
				case parser.EventGameStart:
					gameStepStart = len(steps) - 1
				case parser.EventGameEnd:
					if gameStepStart >= 0 {
						games = append(games, buildSummary(
							len(games), steps, gameStepStart, len(steps), currentFile,
						))
						gameStepStart = -1
					}
				}
			default:
				return
			}
		}
	}

	for fileIdx, path := range paths {
		// Trailing lines of the previous file must not bleed into the first
		// event of this file: fold them into the previous file's last step.
		if fileIdx > 0 {
			foldAccum()
		}
		currentFile = path
		if prog != nil {
			prog.fileIdx.Store(int32(fileIdx))
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		var lineCount int64
		for scanner.Scan() {
			line := scanner.Text()
			// Player.log (macOS) prefixes Power lines with "[Power] ".
			// Strip it so the parser sees the same format as Power.log.
			line = strings.TrimPrefix(line, playerLogPowerPrefix)
			accum = append(accum, line)
			p.Feed(line)
			lineCount++
			if prog != nil && lineCount%1000 == 0 {
				prog.linesRead.Add(1000)
			}
			prevGames := len(games)
			drain()
			if prog != nil && len(games) > prevGames {
				prog.gamesFound.Store(int32(len(games)))
			}
		}
		if prog != nil {
			prog.linesRead.Add(lineCount % 1000)
		}

		if err := scanner.Err(); err != nil {
			f.Close()
			return nil, fmt.Errorf("scanning %s: %w", path, err)
		}
		f.Close()
	}

	// Flush any pending block event.
	p.Flush()
	drain()

	// Fold any trailing raw lines into the last step so they remain visible.
	foldAccum()

	// If a game was started but never ended, capture it as incomplete.
	if gameStepStart >= 0 && gameStepStart < len(steps) {
		games = append(games, buildSummary(
			len(games), steps, gameStepStart, len(steps), currentFile,
		))
	}

	return &Replay{Steps: steps, Games: games, EventCount: eventCount}, nil
}

// Dump parses the log at path, renders the TUI state at the given BG turn to a plain
// string, and returns it. turn=0 means the last step. width controls rendering width.
func Dump(path string, turn int, width int) (string, error) {
	replay, err := LoadReplay(path)
	if err != nil {
		return "", err
	}
	return DumpFromReplay(replay, turn, width)
}

// DumpFromReplay renders a specific turn from an already-loaded replay.
// This avoids re-parsing the log file when rendering multiple turns.
func DumpFromReplay(replay *Replay, turn int, width int) (string, error) {
	if len(replay.Games) == 0 {
		return "", fmt.Errorf("no games found in replay")
	}

	m := NewFromReplay(replay)
	m.width = width
	m.height = 40

	// selectGame(0) is already called by NewFromReplay when there is exactly one game;
	// call it explicitly to handle the multi-game case and ensure picking=false.
	m.selectGame(0)

	if turn == 0 {
		// Jump to last step.
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
	} else {
		m.jumpToTurn(turn)
	}

	return m.View(), nil
}

// LoadAllFromStore builds a Replay with all games from the database that have turn snapshots.
func LoadAllFromStore(st *store.Store) (*Replay, error) {
	metas, err := st.ListGames(0, 0)
	if err != nil {
		return nil, fmt.Errorf("listing games: %w", err)
	}

	var allSteps []Step
	var allGames []GameSummary

	for _, meta := range metas {
		snaps, err := st.GetTurnSnapshots(meta.GameID)
		if err != nil || len(snaps) == 0 {
			continue
		}

		stepStart := len(allSteps)
		for i, snap := range snaps {
			allSteps = append(allSteps, Step{
				Index: stepStart + i,
				Event: parser.GameEvent{
					Type: parser.EventTurnStart,
					Tags: map[string]string{"TURN": fmt.Sprintf("%d", snap.Turn)},
				},
				State: snap.State,
				Turn:  snap.Turn,
			})
		}

		last := snaps[len(snaps)-1]
		allGames = append(allGames, GameSummary{
			Index:      len(allGames),
			PlayerName: last.State.Player.Name,
			HeroCardID: last.State.Player.HeroCardID,
			Placement:  last.State.Placement,
			MaxTurn:    last.Turn,
			TavernTier: last.State.TavernTier,
			Phase:      last.State.Phase,
			StartTime:  last.State.StartTime,
			SourceFile: "(database)",
			StepStart:  stepStart,
			StepEnd:    len(allSteps),
			IsDuos:     last.State.IsDuos,
		})
	}

	return &Replay{Steps: allSteps, Games: allGames, EventCount: len(allSteps)}, nil
}

func buildSummary(idx int, steps []Step, start, end int, file string) GameSummary {
	last := steps[end-1]
	first := steps[start]

	maxTurn := 0
	for _, s := range steps[start:end] {
		if s.Turn > maxTurn {
			maxTurn = s.Turn
		}
	}

	return GameSummary{
		Index:      idx,
		PlayerName: last.State.Player.Name,
		HeroCardID: last.State.Player.HeroCardID,
		Placement:  last.State.Placement,
		MaxTurn:    maxTurn,
		TavernTier: last.State.TavernTier,
		Phase:      last.State.Phase,
		StartTime:  first.Event.Timestamp,
		SourceFile: file,
		StepStart:  start,
		StepEnd:    end,
		IsDuos:     last.State.IsDuos,
	}
}
