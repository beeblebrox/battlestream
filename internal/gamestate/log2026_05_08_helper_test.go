package gamestate_test

// log2026_05_08_helper_test.go provides a shared, lazily-parsed BGGameState for all
// tests in this package that read testdata/power_log_2026_05_08.txt.
//
// This is a Duos BG game (Moch#1358 + LoboSelvagem) ending in a loss (LOST).
// The game is notable for having Lurking Leviathan (BG35_602) on the final board
// and BG35_602e enchantments applied to Beasts.

import (
	"bufio"
	"os"
	"sync"
	"testing"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

const logFile2026_05_08 = "testdata/power_log_2026_05_08.txt"

var (
	log20260508Once  sync.Once
	log20260508State gamestate.BGGameState
	log20260508Skip  string
)

func sharedLog20260508State(t *testing.T) gamestate.BGGameState {
	t.Helper()
	log20260508Once.Do(func() {
		f, err := os.Open(logFile2026_05_08)
		if err != nil {
			log20260508Skip = "test fixture not available: " + err.Error()
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
			log20260508Skip = "scanning log: " + err.Error()
			return
		}
		p.Flush()
		drain()

		log20260508State = m.State()
	})

	if log20260508Skip != "" {
		t.Skip(log20260508Skip)
	}
	return log20260508State
}
