package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
	"battlestream.fixates.io/internal/debugtui"
)

// fakeClient implements daemonClient and records Close calls so tests can
// verify connection-lifecycle behavior without a real gRPC connection.
type fakeClient struct {
	closed int
}

func (f *fakeClient) Close() error {
	f.closed++
	return nil
}

func (f *fakeClient) GetCurrentGame(_ context.Context) (*bspb.GameState, error) {
	return &bspb.GameState{}, nil
}

func (f *fakeClient) GetAggregate(_ context.Context) (*bspb.AggregateStats, error) {
	return &bspb.AggregateStats{}, nil
}

func (f *fakeClient) StreamEvents(_ context.Context) (<-chan *bspb.GameEvent, error) {
	return make(chan *bspb.GameEvent), nil
}

// collectMsgs executes cmd (flattening tea.Batch recursively) and returns all
// produced messages. Commands must not block for this to be usable in tests.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, collectMsgs(sub)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// TestDisconnectedMsg_SingleFlightReconnect is a regression test for the
// reconnect storm (audit H5): when the connection drops, multiple in-flight
// commands (fetchGameCmd, waitForEventCmd, ...) each return disconnectedMsg.
// Only the FIRST one may schedule a reconnect; duplicates must be dropped,
// otherwise N parallel reconnect cycles spawn N clients and N tick chains.
func TestDisconnectedMsg_SingleFlightReconnect(t *testing.T) {
	fc := &fakeClient{}
	m := &Model{
		connState:  stateConnected,
		client:     fc,
		generation: 1,
		ctx:        context.Background(),
	}

	_, cmd := m.Update(disconnectedMsg{err: errors.New("conn lost"), gen: 1})
	if cmd == nil {
		t.Fatal("first disconnectedMsg must schedule a reconnect tick")
	}
	if m.connState != stateDisconnected {
		t.Fatalf("connState = %v, want stateDisconnected", m.connState)
	}
	if fc.closed != 1 {
		t.Fatalf("client closed %d times, want 1", fc.closed)
	}

	// A second in-flight command reports the same connection loss.
	_, cmd = m.Update(disconnectedMsg{err: errors.New("conn lost too"), gen: 1})
	if cmd != nil {
		t.Fatal("duplicate disconnectedMsg while already disconnected must be dropped (no second reconnect scheduled)")
	}

	// And another one while a reconnect attempt is in flight.
	m.connState = stateConnecting
	_, cmd = m.Update(disconnectedMsg{err: errors.New("stale"), gen: 1})
	if cmd != nil {
		t.Fatal("disconnectedMsg while reconnecting must be dropped")
	}
}

// TestDisconnectedMsg_StaleGenerationDropped verifies that a disconnect
// reported by a command from a PREVIOUS connection cannot tear down the
// current (healthy) connection.
func TestDisconnectedMsg_StaleGenerationDropped(t *testing.T) {
	fc := &fakeClient{}
	m := &Model{
		connState:  stateConnected,
		client:     fc,
		generation: 2,
		ctx:        context.Background(),
	}

	_, cmd := m.Update(disconnectedMsg{err: errors.New("old stream closed"), gen: 1})
	if cmd != nil {
		t.Fatal("stale-generation disconnectedMsg must be dropped")
	}
	if m.connState != stateConnected {
		t.Fatal("stale-generation disconnectedMsg must not change connState")
	}
	if fc.closed != 0 {
		t.Fatal("stale-generation disconnectedMsg must not close the current client")
	}
}

// TestDisconnectedMsg_FromConnectAlwaysHandled verifies that a failure of the
// (single-flight) connect attempt itself always schedules the next retry,
// even though the model is not in stateConnected.
func TestDisconnectedMsg_FromConnectAlwaysHandled(t *testing.T) {
	m := &Model{
		connState: stateConnecting,
		ctx:       context.Background(),
	}

	_, cmd := m.Update(disconnectedMsg{err: errors.New("dial failed"), fromConnect: true})
	if cmd == nil {
		t.Fatal("connectCmd failure must schedule a reconnect tick")
	}
	if m.connState != stateDisconnected {
		t.Fatalf("connState = %v, want stateDisconnected", m.connState)
	}
}

// TestConnectedMsg_ClosesPreviousClient is a regression test for the gRPC
// connection leak: connectedMsg used to overwrite m.client without closing
// the previous connection.
func TestConnectedMsg_ClosesPreviousClient(t *testing.T) {
	oldClient := &fakeClient{}
	newClient := &fakeClient{}
	m := &Model{
		connState:  stateConnecting,
		client:     oldClient,
		generation: 1,
		ctx:        context.Background(),
	}

	_, cmd := m.Update(connectedMsg{
		client:  newClient,
		game:    &bspb.GameState{},
		agg:     &bspb.AggregateStats{},
		eventCh: make(chan *bspb.GameEvent),
	})

	if oldClient.closed != 1 {
		t.Fatalf("previous client closed %d times, want 1 (connection leak)", oldClient.closed)
	}
	if newClient.closed != 0 {
		t.Fatal("new client must not be closed")
	}
	if m.client != daemonClient(newClient) {
		t.Fatal("m.client must be the new client")
	}
	if m.generation != 2 {
		t.Fatalf("generation = %d, want 2 (must increment on reconnect)", m.generation)
	}
	if cmd == nil {
		t.Fatal("connectedMsg must start event/tick chains")
	}
}

// TestTickMsgs_StaleGenerationDropped verifies that tick-chain messages from
// a previous connection die instead of re-arming, so after a reconnect
// exactly one agg chain and one game chain survive.
func TestTickMsgs_StaleGenerationDropped(t *testing.T) {
	fc := &fakeClient{}
	m := &Model{
		connState:  stateConnected,
		client:     fc,
		generation: 3,
		ctx:        context.Background(),
	}

	if _, cmd := m.Update(gameTickMsg{gen: 2}); cmd != nil {
		t.Fatal("stale gameTickMsg must not re-arm")
	}
	if _, cmd := m.Update(aggTickMsg{gen: 2}); cmd != nil {
		t.Fatal("stale aggTickMsg must not re-arm")
	}
	if _, cmd := m.Update(eventMsg{gen: 2}); cmd != nil {
		t.Fatal("stale eventMsg must not re-arm a reader on the current channel")
	}

	// Current-generation ticks keep their chains alive.
	if _, cmd := m.Update(gameTickMsg{gen: 3}); cmd == nil {
		t.Fatal("current-generation gameTickMsg must re-arm")
	}
	if _, cmd := m.Update(aggTickMsg{gen: 3}); cmd == nil {
		t.Fatal("current-generation aggTickMsg must re-arm")
	}
}

// TestCombinedSwitchMode_DoesNotAddEventReader is a regression test for the
// duplicated event readers (audit M14): every Tab toggle back to live used to
// arm another waitForEventCmd while the existing chain was still running, so
// N toggles left N+1 concurrent readers.
func TestCombinedSwitchMode_DoesNotAddEventReader(t *testing.T) {
	fc := &fakeClient{}
	// A closed channel makes any armed reader return immediately
	// (as disconnectedMsg), so we can count readers without blocking.
	ch := make(chan *bspb.GameEvent)
	close(ch)

	live := &Model{
		connState:  stateConnected,
		client:     fc,
		eventCh:    ch,
		generation: 1,
		ctx:        context.Background(),
	}
	c := &CombinedModel{
		mode:              modeLive,
		live:              live,
		replayInitialized: true,
	}

	// live -> replay
	if _, cmd := c.Update(tea.KeyMsg{Type: tea.KeyTab}); cmd != nil {
		for _, msg := range collectMsgs(cmd) {
			if _, ok := msg.(disconnectedMsg); ok {
				t.Fatal("switching to replay armed an event reader")
			}
		}
	}

	// replay -> live: must refresh state but NOT arm another reader.
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyTab})
	readers := 0
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(disconnectedMsg); ok {
			readers++
		}
	}
	if readers != 0 {
		t.Fatalf("switching back to live armed %d extra event reader(s); the existing chain already covers events", readers)
	}
}

// TestCombinedReplayMode_BlocksKeyInputToLive is a regression test for input
// leakage (audit M15): in replay mode, unhandled keys used to fall through to
// the hidden live model ('o' opened a browser, 'd'/'l'/'r'/'R' mutated state).
func TestCombinedReplayMode_BlocksKeyInputToLive(t *testing.T) {
	fc := &fakeClient{}
	live := &Model{
		connState:      stateConnected,
		client:         fc,
		game:           &bspb.GameState{Phase: "RECRUIT", AnomalyDescription: "weird anomaly"},
		showLastResult: true,
		generation:     1,
		ctx:            context.Background(),
	}
	replay := debugtui.NewFromReplay(&debugtui.Replay{})
	replay.SetEmbedded(true)
	c := &CombinedModel{
		mode:              modeReplay,
		live:              live,
		replay:            replay,
		replayInitialized: true,
	}

	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if !live.showLastResult {
		t.Fatal("key 'l' leaked into the hidden live model (toggled showLastResult)")
	}

	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if live.showAnomalyDesc {
		t.Fatal("key 'd' leaked into the hidden live model (toggled showAnomalyDesc)")
	}
}

// TestCombinedReplayMode_BlocksMouseInputToLive verifies that mouse events in
// replay mode never reach the live model (drags used to move the invisible
// live divider and fire config saves).
func TestCombinedReplayMode_BlocksMouseInputToLive(t *testing.T) {
	fc := &fakeClient{}
	live := &Model{
		connState:  stateConnected,
		client:     fc,
		game:       &bspb.GameState{Phase: "RECRUIT"},
		width:      120,
		height:     40,
		generation: 1,
		ctx:        context.Background(),
	}
	// Position the vertical divider so a press at (10, 5) would start a drag.
	live.dividerX = 10
	live.row2StartY = 0

	replay := debugtui.NewFromReplay(&debugtui.Replay{})
	replay.SetEmbedded(true)
	c := &CombinedModel{
		mode:              modeReplay,
		live:              live,
		replay:            replay,
		replayInitialized: true,
	}

	c.Update(tea.MouseMsg{
		X:      10,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if live.draggingV {
		t.Fatal("mouse press leaked into the hidden live model (started divider drag)")
	}
}

// TestCombinedReplayMode_ForwardsBackgroundMsgsToLive verifies the whitelist:
// connection/data/tick messages must still reach the hidden live model so its
// chains stay alive and state stays fresh while the replay view is shown.
func TestCombinedReplayMode_ForwardsBackgroundMsgsToLive(t *testing.T) {
	fc := &fakeClient{}
	live := &Model{
		connState:  stateConnected,
		client:     fc,
		generation: 1,
		ctx:        context.Background(),
	}
	replay := debugtui.NewFromReplay(&debugtui.Replay{})
	replay.SetEmbedded(true)
	c := &CombinedModel{
		mode:              modeReplay,
		live:              live,
		replay:            replay,
		replayInitialized: true,
	}

	game := &bspb.GameState{GameId: "bg-game-42"}
	c.Update(gameUpdateMsg{game: game})
	if live.game == nil || live.game.GameId != "bg-game-42" {
		t.Fatal("gameUpdateMsg must reach the hidden live model in replay mode")
	}

	_, cmd := c.Update(gameTickMsg{gen: 1})
	if cmd == nil {
		t.Fatal("gameTickMsg must reach the hidden live model and re-arm its chain")
	}
}
