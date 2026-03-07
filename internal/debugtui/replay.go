// Package debugtui implements a step-by-step Power.log replay TUI for debugging.
package debugtui

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

// Step represents a single parsed event with its associated state snapshot.
type Step struct {
	Index    int
	Event    parser.GameEvent
	RawLines []string              // log lines that produced this event
	State    gamestate.BGGameState // snapshot AFTER processing this event
	Turn     int                   // BG turn number at this point
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
}

// Replay holds the complete parsed result: all steps and game summaries.
type Replay struct {
	Steps []Step
	Games []GameSummary
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

	var steps []Step
	var games []GameSummary
	var accum []string
	var currentFile string

	// Track current game boundary.
	gameStepStart := -1

	drain := func() {
		for {
			select {
			case evt := <-ch:
				proc.Handle(evt)
				snap := machine.State()
				steps = append(steps, Step{
					Index:    len(steps),
					Event:    evt,
					RawLines: accum,
					State:    snap,
					Turn:     snap.Turn,
				})
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

	// If a game was started but never ended, capture it as incomplete.
	if gameStepStart >= 0 && gameStepStart < len(steps) {
		games = append(games, buildSummary(
			len(games), steps, gameStepStart, len(steps), currentFile,
		))
	}

	return &Replay{Steps: steps, Games: games}, nil
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
	}
}
