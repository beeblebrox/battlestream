package capture

import (
	"context"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
	"battlestream.fixates.io/internal/watcher"
)

// logEventSource wraps watcher + parser into an EventSource.
type logEventSource struct {
	watcher *watcher.Watcher
	parser  *parser.Parser
	events  chan parser.GameEvent
	cancel  context.CancelFunc
}

// NewEventSource creates an EventSource that tails Power.log and emits parsed events.
func NewEventSource(ctx context.Context, powerLogDir string) (EventSource, error) {
	events := make(chan parser.GameEvent, 256)
	p := parser.New(events)

	ctx, cancel := context.WithCancel(ctx)
	w, err := watcher.New(ctx, watcher.Config{
		LogDir: powerLogDir,
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
	go func() {
		for line := range w.Lines {
			p.Feed(line.Text)
		}
		p.Flush()
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
	machine   *gamestate.Machine
	processor *gamestate.Processor
}

// NewStateTracker creates a StateTracker backed by gamestate.Machine/Processor.
func NewStateTracker() StateTracker {
	m := gamestate.New()
	p := gamestate.NewProcessor(m)
	return &machineStateTracker{machine: m, processor: p}
}

func (t *machineStateTracker) Apply(event parser.GameEvent) {
	t.processor.Handle(event)
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
	return phase != gamestate.PhaseIdle && phase != gamestate.PhaseLobby
}
