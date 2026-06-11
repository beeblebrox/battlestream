package capture

import (
	"context"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/gamestate/builder"
	"battlestream.fixates.io/internal/gamestate/card"
	"battlestream.fixates.io/internal/parser"
	"battlestream.fixates.io/internal/watcher"
)

// catchupQuiescence is the fallback window for declaring the startup backlog
// drained when the log's last line predates startup and nothing new is being
// written. The tail reads pre-existing content at disk speed, so if the feed
// goroutine is starved of lines for this long, the tail can only be parked at
// EOF waiting for new writes — i.e. we have caught up. (Channel backpressure
// cannot starve the receiver: a full channel hands a line over immediately.)
const catchupQuiescence = 2 * time.Second

// logEventSource wraps watcher + parser into an EventSource.
type logEventSource struct {
	watcher *watcher.Watcher
	parser  *parser.Parser
	events  chan parser.GameEvent
	cancel  context.CancelFunc
}

// NewEventSource creates an EventSource that tails Power.log and emits parsed events.
func NewEventSource(ctx context.Context, powerLogDir string) (EventSource, error) {
	events := make(chan parser.GameEvent, 16384)
	p := parser.New(events)

	ctx, cancel := context.WithCancel(ctx)
	w, err := watcher.New(ctx, watcher.Config{
		LogDir: powerLogDir,
		// Power.log only: the parser ignores everything else anyway, and the
		// catchup barrier below must not be tripped by unrelated files.
		Files:         []string{"Power.log"},
		ReadFromStart: true,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	src := &logEventSource{
		watcher: w,
		parser:  p,
		events:  events,
		cancel:  cancel,
	}

	// Feed watcher lines to parser in background.
	//
	// Catchup barrier: ReadFromStart replays the whole Power.log, which may
	// contain games that finished long ago. The capture loop must not start
	// a session until that backlog is drained, or it would screenshot the
	// current desktop into a historical game (audit finding H6). We emit a
	// synthetic EventCatchupComplete once the backlog ends, detected by
	// whichever comes first:
	//
	//  1. The first line the watcher marks as live (Line.Backlog=false,
	//     a mechanical byte-offset boundary recorded at tail start). This is
	//     the prompt path when Hearthstone is actively writing — e.g. a game
	//     is in progress at startup, which we still want to capture.
	//  2. catchupQuiescence elapsing with no lines at all — the quiet-tail
	//     path when the log ends with old content and nothing new arrives.
	//
	// Both signals are injected by this goroutine, the sole producer on the
	// events channel, and parser.Feed emits synchronously — so the marker is
	// strictly ordered after every event derived from backlog lines. The
	// loop flips its barrier only when the consumer dequeues the marker,
	// i.e. after all backlog events have been applied to the tracker.
	go func() {
		defer p.Flush()
		caughtUp := false
		markCaughtUp := func() {
			caughtUp = true
			events <- parser.GameEvent{Type: EventCatchupComplete}
		}
		for {
			if caughtUp {
				line, ok := <-w.Lines
				if !ok {
					return
				}
				p.Feed(line.Text)
				continue
			}
			select {
			case line, ok := <-w.Lines:
				if !ok {
					return
				}
				if !line.Backlog {
					// Marker first: this line is live, its events belong
					// after the barrier opens.
					markCaughtUp()
				}
				p.Feed(line.Text)
			case <-time.After(catchupQuiescence):
				markCaughtUp()
			}
		}
	}()

	return src, nil
}

func (s *logEventSource) Events() <-chan parser.GameEvent {
	return s.events
}

func (s *logEventSource) Close() error {
	s.cancel()
	s.watcher.Stop()
	return nil
}

// machineStateTracker wraps gamestate.Machine + Processor into a StateTracker.
type machineStateTracker struct {
	machine    *gamestate.Machine
	processor  *gamestate.Processor
	builder    *builder.ActionBuilder
	dispatcher *gamestate.ActionDispatcher
}

// NewStateTracker creates a StateTracker backed by gamestate.Machine/Processor.
func NewStateTracker() StateTracker {
	m := gamestate.New()
	p := gamestate.NewProcessor(m)
	catalog := card.NewFuncCatalog(gamestate.CardName)
	bldr := builder.New(catalog)
	disp := gamestate.NewActionDispatcher(
		p.AsRecruitVisitor(),
		p.AsCombatVisitor(),
		p.AsTransitionVisitor(),
	)
	return &machineStateTracker{machine: m, processor: p, builder: bldr, dispatcher: disp}
}

func (t *machineStateTracker) Apply(event parser.GameEvent) {
	if a := t.builder.Build(event); a != nil {
		_ = t.dispatcher.Dispatch(a)
	} else {
		t.processor.Handle(event)
	}
}

func (t *machineStateTracker) Snapshot() CaptureState {
	s := t.machine.State() // acquires RLock internally, returns deep copy
	cs := CaptureState{
		GameID:     s.GameID,
		Timestamp:  time.Now(),
		Turn:       s.Turn,
		Phase:      string(s.Phase),
		TavernTier: s.TavernTier,
		Health:     s.Player.Health,
		Armor:      s.Player.Armor,
		Gold:       s.Player.CurrentGold,
		Placement:  s.Placement,
		IsDuos:     s.IsDuos,
	}
	if s.Partner != nil {
		cs.PartnerHealth = s.Partner.Health
		cs.PartnerTier = s.Partner.TavernTier
	}
	for _, m := range s.Board {
		cs.Board = append(cs.Board, MinionSnapshot{
			CardID:     m.CardID,
			Name:       m.Name,
			Attack:     m.Attack,
			Health:     m.Health,
			Tribes:     m.MinionType,
			BuffAttack: m.BuffAttack,
			BuffHealth: m.BuffHealth,
		})
	}
	for _, b := range s.BuffSources {
		cs.BuffSources = append(cs.BuffSources, BuffSourceSnapshot{
			Category: b.Category,
			Attack:   b.Attack,
			Health:   b.Health,
		})
	}
	return cs
}

func (t *machineStateTracker) InGame() bool {
	phase := t.machine.Phase()
	return phase != gamestate.PhaseIdle && phase != gamestate.PhaseLobby && phase != gamestate.PhaseGameOver
}
