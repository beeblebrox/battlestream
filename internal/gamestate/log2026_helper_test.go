package gamestate_test

// log2026_helper_test.go provides a shared, lazily-parsed BGGameState for all
// tests in this file that read testdata/power_log_2026_03_07.txt.
//
// Without this, each Test* function calls parseLog2026_03_07 independently,
// causing the 593K-line file to be parsed 8+ times. Under the race detector
// (~10-20x slower) that exceeds the default 120s test timeout.

import (
	"bufio"
	"os"
	"sync"
	"testing"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

var (
	log2026Once  sync.Once
	log2026State gamestate.BGGameState
	log2026Skip  string // non-empty means t.Skip with this message
)

// sharedLog2026State returns the parsed BGGameState from the 2026-03-07 log,
// parsing it at most once for the entire test binary lifetime.
func sharedLog2026State(t *testing.T) gamestate.BGGameState {
	t.Helper()
	log2026Once.Do(func() {
		f, err := os.Open(logFile2026_03_07)
		if err != nil {
			log2026Skip = "test fixture not available: " + err.Error()
			return
		}
		defer f.Close()

		ch := make(chan parser.GameEvent, 1024)
		p := parser.New(ch)
		m := gamestate.New()
		proc := gamestate.NewProcessor(m)

		drain := func() {
			for {
				select {
				case e := <-ch:
					proc.Handle(e)
				default:
					return
				}
			}
		}

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			p.Feed(scanner.Text())
			drain()
		}
		if err := scanner.Err(); err != nil {
			log2026Skip = "scanning log: " + err.Error()
			return
		}
		p.Flush()
		drain()

		log2026State = m.State()
	})

	if log2026Skip != "" {
		t.Skip(log2026Skip)
	}
	return log2026State
}
