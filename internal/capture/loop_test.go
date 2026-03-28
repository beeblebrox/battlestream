package capture

import (
	"context"
	"image"
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
	return testImage(1920, 1080), nil
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

func TestLoopCapturesDuringGame(t *testing.T) {
	events := newMockEventSource()
	tracker := &mockStateTracker{}
	store := &mockFrameStore{}

	loop := NewLoop(events, tracker, &mockScreenshotter{}, store,
		50*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	go loop.Run(ctx)

	// Simulate game start.
	tracker.SetInGame(true, "test-game")
	events.Send(parser.GameEvent{Type: parser.EventGameStart})

	// Wait for a few captures.
	time.Sleep(200 * time.Millisecond)

	if store.FrameCount() == 0 {
		t.Error("expected frames to be captured during game")
	}

	// Simulate game end.
	tracker.SetInGame(false, "")
	time.Sleep(100 * time.Millisecond)

	cancel()
	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	finalized := store.finalized
	store.mu.Unlock()

	if !finalized {
		t.Error("expected game to be finalized after end")
	}
}
