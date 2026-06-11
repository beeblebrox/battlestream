package capture

import (
	"context"
	"log/slog"
	"time"
)

// Loop is the main capture loop orchestrator.
type Loop struct {
	events        EventSource
	tracker       StateTracker
	screenshotter Screenshotter
	store         FrameStore
	interval      time.Duration
	staleTimeout  time.Duration
}

// NewLoop creates a capture loop with the given components.
func NewLoop(
	events EventSource,
	tracker StateTracker,
	screenshotter Screenshotter,
	store FrameStore,
	interval time.Duration,
	staleTimeout time.Duration,
) *Loop {
	return &Loop{
		events:        events,
		tracker:       tracker,
		screenshotter: screenshotter,
		store:         store,
		interval:      interval,
		staleTimeout:  staleTimeout,
	}
}

// screenshotFailureLogEvery throttles repeated screenshot-failure logging:
// the first failure is logged, then one in every N consecutive failures.
// At a 1s capture interval that is roughly one log line per 30 seconds
// instead of one per tick.
const screenshotFailureLogEvery = 30

// finishGame finalizes the current capture session and closes the store,
// logging (rather than discarding) any errors.
func (l *Loop) finishGame(placement int) {
	if err := l.store.FinalizeGame(placement); err != nil {
		slog.Error("failed to finalize game", "err", err, "placement", placement)
	}
	if err := l.store.Close(); err != nil {
		slog.Error("failed to close frame store", "err", err)
	}
}

// consumeEvents applies events to the state tracker in order, signals event
// freshness on eventTimeCh, and forwards the catchup marker on caughtUpCh.
// Because events are applied in order here, the catchup signal is only raised
// after every backlog event has reached the tracker (no race against the
// 16K-event channel buffer). Returns when ctx is cancelled or the event
// source closes.
func (l *Loop) consumeEvents(ctx context.Context, eventTimeCh chan<- time.Time, caughtUpCh chan<- struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-l.events.Events():
			if !ok {
				return
			}
			if ev.Type == EventCatchupComplete {
				select {
				case caughtUpCh <- struct{}{}:
				default:
				}
				continue
			}
			l.tracker.Apply(ev)
			// Freshness signal only — never block on it. A blocking send
			// here could wedge this goroutine forever once Run has exited
			// (nothing drains the channel after that); dropping is fine
			// because a pending timestamp is already queued.
			select {
			case eventTimeCh <- time.Now():
			case <-ctx.Done():
				return
			default:
			}
		}
	}
}

// Run starts the capture loop. Blocks until ctx is cancelled.
func (l *Loop) Run(ctx context.Context) error {
	var (
		lastEventTime = time.Now()
		capturing     = false
		sequence      = 0
		// caughtUp gates session starts: until the event source signals
		// EventCatchupComplete, any "in game" state may come from replaying
		// historical log content, and screenshotting the current desktop
		// into a game that ended hours ago would corrupt the archive.
		caughtUp      = false
		waitLogged    = false // logged the pre-catchup deferral for the current game
		emptyIDLogged = false // logged the empty-GameID skip for the current game
		// Consecutive screenshot failures; throttles error logging so a
		// persistently broken screenshotter (e.g. grim missing) does not
		// emit one error line per tick.
		captureFailures = 0
	)

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	eventTimeCh := make(chan time.Time, 256)
	caughtUpCh := make(chan struct{}, 1)
	go l.consumeEvents(ctx, eventTimeCh, caughtUpCh)

	setCaughtUp := func() {
		if !caughtUp {
			caughtUp = true
			slog.Info("log replay caught up to live tail, capture enabled")
		}
	}

	for {
		select {
		case <-ctx.Done():
			if capturing {
				snap := l.tracker.Snapshot()
				l.finishGame(snap.Placement)
			}
			return ctx.Err()

		case t := <-eventTimeCh:
			lastEventTime = t

		case <-caughtUpCh:
			setCaughtUp()

		case <-ticker.C:
			// Drain any pending event times and catchup signal.
			for {
				select {
				case t := <-eventTimeCh:
					lastEventTime = t
				case <-caughtUpCh:
					setCaughtUp()
				default:
					goto donedraining
				}
			}
		donedraining:

			inGame := l.tracker.InGame()

			// Check stale timeout.
			if capturing && time.Since(lastEventTime) > l.staleTimeout {
				slog.Warn("stale timeout reached, ending capture",
					"elapsed", time.Since(lastEventTime))
				snap := l.tracker.Snapshot()
				l.finishGame(snap.Placement)
				capturing = false
				sequence = 0
				continue
			}

			if !inGame {
				// Reset once-per-game log dedup so the next game logs again.
				waitLogged = false
				emptyIDLogged = false
			}

			// Game started.
			if inGame && !capturing {
				// Catchup barrier: during backlog replay the tracker may be
				// inside a historical, already-finished game. Defer until the
				// source confirms we are at the live tail; a game genuinely
				// in progress at startup is still captured then (the tracker
				// keeps reporting it in-game once replay completes).
				if !caughtUp {
					if !waitLogged {
						slog.Info("game state active during log catchup, deferring capture until replay reaches live tail")
						waitLogged = true
					}
					continue
				}
				snap := l.tracker.Snapshot()
				// Wait until GameID is populated to avoid creating files at
				// the root data dir.
				if snap.GameID == "" {
					if !emptyIDLogged {
						slog.Warn("in game but GameID empty, deferring capture start",
							"phase", snap.Phase)
						emptyIDLogged = true
					}
					continue
				}
				slog.Info("game detected, starting capture", "game_id", snap.GameID)
				if err := l.store.InitGame(snap.GameID); err != nil {
					slog.Error("failed to init game store", "err", err)
					continue
				}
				capturing = true
				sequence = 0
				lastEventTime = time.Now()
				waitLogged = false
				emptyIDLogged = false
			}

			// Game ended.
			if !inGame && capturing {
				snap := l.tracker.Snapshot()
				slog.Info("game ended", "placement", snap.Placement)
				l.finishGame(snap.Placement)
				capturing = false
				sequence = 0
				continue
			}

			// Capture frame.
			if capturing {
				captureStart := time.Now()
				snap := l.tracker.Snapshot()

				img, err := l.screenshotter.Capture(ctx)
				if err != nil {
					captureFailures++
					if captureFailures == 1 || captureFailures%screenshotFailureLogEvery == 0 {
						slog.Error("screenshot failed", "err", err,
							"consecutive_failures", captureFailures)
					}
					continue
				}
				if captureFailures > 0 {
					slog.Info("screenshot recovered", "after_failures", captureFailures)
					captureFailures = 0
				}

				latency := time.Since(captureStart).Milliseconds()
				if latency > l.interval.Milliseconds() {
					slog.Warn("capture took longer than interval",
						"latency_ms", latency, "interval_ms", l.interval.Milliseconds())
				}

				frame := Frame{
					Sequence:       sequence,
					Image:          img,
					State:          snap,
					CaptureLatency: latency,
				}

				if err := l.store.SaveFrame(frame); err != nil {
					slog.Error("save frame failed", "err", err, "seq", sequence)
					continue
				}

				sequence++
			}
		}
	}
}
