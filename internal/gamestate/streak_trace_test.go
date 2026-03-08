package gamestate_test

import (
	"bufio"
	"fmt"
	"os"
	"testing"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

func TestStreakTrace(t *testing.T) {
	f, err := os.Open("testdata/power_log_2026_03_07.txt")
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()

	ch := make(chan parser.GameEvent, 1024)
	p := parser.New(ch)
	m := gamestate.New()
	proc := gamestate.NewProcessor(m)

	prevTurn := 0
	drain := func() {
		for {
			select {
			case e := <-ch:
				proc.Handle(e)
				s := m.State()
				if s.Turn != prevTurn {
					fmt.Printf("Turn %2d  Phase=%-10s  W=%d L=%d  Armor=%d\n",
						s.Turn, s.Phase, s.Player.WinStreak, s.Player.LossStreak, s.Player.Armor)
					prevTurn = s.Turn
				}
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
	p.Flush()
	drain()

	s := m.State()
	fmt.Printf("\nFinal: Phase=%s W=%d L=%d Armor=%d\n", s.Phase, s.Player.WinStreak, s.Player.LossStreak, s.Player.Armor)
}
