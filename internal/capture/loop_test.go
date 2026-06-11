package capture

import (
	"context"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"battlestream.fixates.io/internal/parser"
)

// mockEventSource emits events sent via Send().
type mockEventSource struct {
	ch chan parser.GameEvent
}

func newMockEventSource() *mockEventSource {
	return &mockEventSource{ch: make(chan parser.GameEvent, 64)}
}
func (m *mockEventSource) Events() <-chan parser.GameEvent { return m.ch }
func (m *mockEventSource) Close() error                   { close(m.ch); return nil }
func (m *mockEventSource) Send(e parser.GameEvent)         { m.ch <- e }

// mockStateTracker returns canned state.
type mockStateTracker struct {
	mu     sync.Mutex
	state  CaptureState
	inGame bool
}

func (m *mockStateTracker) Apply(_ parser.GameEvent) {}
func (m *mockStateTracker) Snapshot() CaptureState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}
func (m *mockStateTracker) InGame() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inGame
}
func (m *mockStateTracker) SetInGame(v bool, gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inGame = v
	m.state.GameID = gameID
	if v {
		m.state.Phase = "RECRUIT"
	} else {
		m.state.Phase = "IDLE"
	}
}

// mockScreenshotter returns a test image.
type mockScreenshotter struct{}

func (m *mockScreenshotter) Capture(_ context.Context) (image.Image, error) {
	return testImage(4, 4), nil // tiny image for fast tests
}

// mockFrameStore records calls.
type mockFrameStore struct {
	mu        sync.Mutex
	initCalls []string
	frames    []Frame
	finalized bool
	placement int
}

func (m *mockFrameStore) InitGame(gameID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCalls = append(m.initCalls, gameID)
	return nil
}
func (m *mockFrameStore) SaveFrame(f Frame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frames = append(m.frames, f)
	return nil
}
func (m *mockFrameStore) FinalizeGame(placement int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalized = true
	m.placement = placement
	return nil
}
func (m *mockFrameStore) Close() error { return nil }
func (m *mockFrameStore) FrameCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.frames)
}
func (m *mockFrameStore) InitCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.initCalls...)
}

func TestLoopCapturesDuringGame(t *testing.T) {
	events := newMockEventSource()
	tracker := &mockStateTracker{}
	store := &mockFrameStore{}

	loop := NewLoop(events, tracker, &mockScreenshotter{}, store,
		100*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	go loop.Run(ctx)

	// The source is at the live tail (no backlog) — signal catchup first,
	// per the EventSource contract.
	events.Send(parser.GameEvent{Type: EventCatchupComplete})

	// Simulate game start.
	tracker.SetInGame(true, "test-game")
	events.Send(parser.GameEvent{Type: parser.EventGameStart})

	// Wait for a few captures (generous timing for -race).
	time.Sleep(500 * time.Millisecond)

	if store.FrameCount() == 0 {
		t.Error("expected frames to be captured during game")
	}

	// Simulate game end.
	tracker.SetInGame(false, "")
	time.Sleep(200 * time.Millisecond)

	cancel()
	time.Sleep(100 * time.Millisecond)

	store.mu.Lock()
	finalized := store.finalized
	store.mu.Unlock()

	if !finalized {
		t.Error("expected game to be finalized after end")
	}
}

// waitFor polls cond until it returns true or the deadline expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return cond()
}

// TestLoopDefersCaptureUntilCatchup is the regression test for audit finding
// H6: during backlog replay of Power.log, the tracker can be "in game" inside
// a historical, already-finished game. The loop must not start a capture
// session (and screenshot the current desktop into that old game) until the
// event source signals that replay has caught up to the live tail.
func TestLoopDefersCaptureUntilCatchup(t *testing.T) {
	events := newMockEventSource()
	tracker := &mockStateTracker{}
	store := &mockFrameStore{}

	loop := NewLoop(events, tracker, &mockScreenshotter{}, store,
		20*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// Backlog replay is positioned inside a historical game: in-game with the
	// old game's ID, but no catchup signal yet.
	tracker.SetInGame(true, "historical-game")
	time.Sleep(300 * time.Millisecond) // ~15 ticks

	if calls := store.InitCalls(); len(calls) != 0 {
		t.Fatalf("capture session started during backlog replay of a historical game: %v", calls)
	}
	if n := store.FrameCount(); n != 0 {
		t.Fatalf("frames captured during backlog replay: %d", n)
	}

	// Replay finishes: the historical game completes, then the source
	// signals catchup.
	tracker.SetInGame(false, "")
	events.Send(parser.GameEvent{Type: EventCatchupComplete})
	time.Sleep(150 * time.Millisecond)

	if calls := store.InitCalls(); len(calls) != 0 {
		t.Fatalf("capture session started for finished historical game after catchup: %v", calls)
	}

	// A live game starts after catchup — this one must be captured.
	tracker.SetInGame(true, "live-game")
	ok := waitFor(t, 2*time.Second, func() bool {
		calls := store.InitCalls()
		return len(calls) == 1 && calls[0] == "live-game" && store.FrameCount() > 0
	})
	if !ok {
		t.Fatalf("live game not captured after catchup: initCalls=%v frames=%d",
			store.InitCalls(), store.FrameCount())
	}
}

// syncBuffer is a goroutine-safe bytes buffer for capturing log output.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestLoopLogsEmptyGameIDSkip asserts that the loop emits an observable log
// line (once per state change, not once per tick) when it skips capture
// because the tracker is in-game but the GameID is still empty.
func TestLoopLogsEmptyGameIDSkip(t *testing.T) {
	var buf syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	events := newMockEventSource()
	tracker := &mockStateTracker{}
	store := &mockFrameStore{}

	loop := NewLoop(events, tracker, &mockScreenshotter{}, store,
		20*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	events.Send(parser.GameEvent{Type: EventCatchupComplete})

	// In game, but GameID not yet populated (CREATE_GAME mid-parse).
	tracker.SetInGame(true, "")
	time.Sleep(300 * time.Millisecond) // many ticks

	if calls := store.InitCalls(); len(calls) != 0 {
		t.Fatalf("capture must not start with empty GameID: %v", calls)
	}
	logs := buf.String()
	count := strings.Count(logs, "GameID empty")
	if count == 0 {
		t.Fatal("expected a log line for the empty-GameID capture skip, got none")
	}
	if count != 1 {
		t.Errorf("empty-GameID skip should log once per state change, got %d lines", count)
	}

	// GameID becomes available — capture starts.
	tracker.SetInGame(true, "now-known")
	ok := waitFor(t, 2*time.Second, func() bool {
		calls := store.InitCalls()
		return len(calls) == 1 && calls[0] == "now-known"
	})
	if !ok {
		t.Fatalf("capture did not start once GameID became available: %v", store.InitCalls())
	}
}

// TestLoopIgnoresHistoricalGameInBacklog is the end-to-end regression test
// for audit finding H6: a Power.log containing a complete, finished game is
// replayed through the real pipeline (watcher -> parser -> tracker) with a
// quiet tail. No capture session may be started for the historical game.
// After catchup, a new game appended to the log must be captured.
func TestLoopIgnoresHistoricalGameInBacklog(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: replays a full Power.log")
	}

	src, err := os.ReadFile(filepath.Join("..", "gamestate", "testdata", "power_log_game.txt"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "Power.log")
	if err := os.WriteFile(logPath, src, 0o644); err != nil {
		t.Fatalf("write Power.log: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := NewEventSource(ctx, dir)
	if err != nil {
		t.Fatalf("NewEventSource: %v", err)
	}
	defer events.Close()

	tracker := NewStateTracker()
	store := &mockFrameStore{}
	loop := NewLoop(events, tracker, &mockScreenshotter{}, store,
		20*time.Millisecond, time.Minute)
	go loop.Run(ctx)

	// Wait for the historical game to be fully replayed (it ends COMPLETE,
	// so the tracker leaves in-game state) ...
	replayed := waitFor(t, 60*time.Second, func() bool {
		return tracker.Snapshot().GameID != "" && !tracker.InGame()
	})
	if !replayed {
		t.Fatalf("historical game never finished replaying: gameID=%q inGame=%v",
			tracker.Snapshot().GameID, tracker.InGame())
	}
	historicalID := tracker.Snapshot().GameID

	// ... then ride out the quiescence window so the catchup marker fires.
	time.Sleep(catchupQuiescence + time.Second)

	if calls := store.InitCalls(); len(calls) != 0 {
		t.Fatalf("capture session started for historical game during replay: %v", calls)
	}
	if n := store.FrameCount(); n != 0 {
		t.Fatalf("frames captured for historical game: %d", n)
	}

	// A new game starts in the live tail — must be captured.
	newGame := strings.Join([]string{
		"D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     Player EntityID=2 PlayerID=1 GameAccountId=[hi=987654 lo=12345]",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     Player EntityID=3 PlayerID=2 GameAccountId=[hi=0 lo=0]",
		"D 10:00:01.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=TURN value=1",
		"",
	}, "\n")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	if _, err := f.WriteString(newGame); err != nil {
		f.Close()
		t.Fatalf("append new game: %v", err)
	}
	f.Close()

	ok := waitFor(t, 10*time.Second, func() bool {
		calls := store.InitCalls()
		return len(calls) == 1 && calls[0] != historicalID && store.FrameCount() > 0
	})
	if !ok {
		t.Fatalf("live game after catchup was not captured: initCalls=%v frames=%d (historical=%s)",
			store.InitCalls(), store.FrameCount(), historicalID)
	}
}
