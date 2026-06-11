package debugtui

import (
	"bufio"
	"os"
	"reflect"
	"runtime"
	"testing"

	"battlestream.fixates.io/internal/parser"
)

// measureReplayLoad loads the 92K-line testdata log and reports retained heap.
// It bypasses the shared replay cache deliberately: the point is to measure
// what one full load retains.
func measureReplayLoad(t *testing.T) (replay *Replay, retainedBytes uint64) {
	t.Helper()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	replay, err := LoadAllGames([]string{testLog})
	if err != nil {
		t.Fatalf("LoadAllGames: %v", err)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if after.HeapAlloc > before.HeapAlloc {
		retainedBytes = after.HeapAlloc - before.HeapAlloc
	}
	runtime.KeepAlive(replay)
	return replay, retainedBytes
}

func countLogLines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return n
}

// TestLoadReplay_MemoryAndDedup quantifies the memory held by a loaded Replay
// and asserts the snapshot-dedup bounds hold (audit H7).
//
// Before the fix, the builder stored a full deep-copied BGGameState per event:
// on the 92,895-line testdata log that was 42,226 steps retaining ~441 MB.
// With no-change events collapsed into the previous step and raw-line
// retention capped per step, the same log yields ~1,275 steps and ~23 MB.
// The assertion thresholds below are deliberately loose (10x step margin,
// ~4x memory margin) so the test catches a regression to per-event snapshots
// without being flaky.
func TestLoadReplay_MemoryAndDedup(t *testing.T) {
	replay, retained := measureReplayLoad(t)

	totalRaw := 0
	totalDropped := 0
	maxRaw := 0
	for _, s := range replay.Steps {
		totalRaw += len(s.RawLines)
		totalDropped += s.RawLinesDropped
		if len(s.RawLines) > maxRaw {
			maxRaw = len(s.RawLines)
		}
	}

	t.Logf("events processed:   %d", replay.EventCount)
	t.Logf("steps retained:     %d", len(replay.Steps))
	t.Logf("raw lines retained: %d (+%d truncated, max per step: %d)", totalRaw, totalDropped, maxRaw)
	t.Logf("retained heap:      %.1f MB", float64(retained)/(1<<20))

	// Sanity: the log actually produces a large event stream.
	if replay.EventCount < 10000 {
		t.Fatalf("expected >= 10000 events from the 92K-line log, got %d", replay.EventCount)
	}

	// Dedup: most events change nothing visible, so the retained step count
	// must be a small fraction of the event count (measured: 1275 / 42226).
	if len(replay.Steps) >= replay.EventCount/10 {
		t.Errorf("dedup ineffective: %d steps for %d events (want < %d)",
			len(replay.Steps), replay.EventCount, replay.EventCount/10)
	}

	// Raw-line cap: no step may exceed maxRawLinesPerStep.
	if maxRaw > maxRawLinesPerStep {
		t.Errorf("raw-line cap violated: step holds %d lines (cap %d)", maxRaw, maxRawLinesPerStep)
	}

	// Accounting: every scanned log line is either retained in some step's
	// RawLines or counted in its RawLinesDropped — none silently vanish.
	if lineCount := countLogLines(t, testLog); totalRaw+totalDropped != lineCount {
		t.Errorf("raw line accounting: retained %d + dropped %d = %d, want %d",
			totalRaw, totalDropped, totalRaw+totalDropped, lineCount)
	}

	// Dedup invariant: consecutive steps must differ — equal neighbours mean
	// a redundant snapshot was retained.
	for i := 1; i < len(replay.Steps); i++ {
		prev, cur := &replay.Steps[i-1], &replay.Steps[i]
		if !isBoundaryEvent(cur) && reflect.DeepEqual(prev.State, cur.State) {
			t.Errorf("steps %d and %d have identical states (event %s not collapsed)",
				i-1, i, cur.Event.Type)
			break
		}
	}

	// Memory: pre-fix this load retained ~441 MB; post-fix ~23 MB. 100 MB is
	// a generous bound that still fails decisively on a regression.
	const maxRetained = 100 << 20
	if retained > maxRetained {
		t.Errorf("retained heap %.1f MB exceeds %d MB bound (pre-fix baseline was ~441 MB)",
			float64(retained)/(1<<20), maxRetained>>20)
	}
}

// isBoundaryEvent reports whether the step was force-appended regardless of
// state equality (game start/end steps anchor game summaries).
func isBoundaryEvent(s *Step) bool {
	return s.Event.Type == parser.EventGameStart || s.Event.Type == parser.EventGameEnd
}
