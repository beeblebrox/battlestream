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

// Run starts the capture loop. Blocks until ctx is cancelled.
func (l *Loop) Run(ctx context.Context) error {
	var (
		lastEventTime = time.Now()
		capturing     = false
		sequence      = 0
	)

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	// Event consumer goroutine — applies events to state tracker,
	// updates last event time.
	eventTimeCh := make(chan time.Time, 256)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-l.events.Events():
				if !ok {
					return
				}
				l.tracker.Apply(ev)
				eventTimeCh <- time.Now()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			if capturing {
				snap := l.tracker.Snapshot()
				l.store.FinalizeGame(snap.Placement)
				l.store.Close()
			}
			return ctx.Err()

		case t := <-eventTimeCh:
			lastEventTime = t

		case <-ticker.C:
			// Drain any pending event times.
			for {
				select {
				case t := <-eventTimeCh:
					lastEventTime = t
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
				l.store.FinalizeGame(snap.Placement)
				l.store.Close()
				capturing = false
				sequence = 0
				continue
			}

			// Game started — wait until GameID is populated to avoid
			// creating files at the root data dir.
			if inGame && !capturing {
				snap := l.tracker.Snapshot()
				if snap.GameID == "" {
					continue // processor hasn't finished CREATE_GAME yet
				}
				slog.Info("game detected, starting capture", "game_id", snap.GameID)
				if err := l.store.InitGame(snap.GameID); err != nil {
					slog.Error("failed to init game store", "err", err)
					continue
				}
				capturing = true
				sequence = 0
				lastEventTime = time.Now()
			}

			// Game ended.
			if !inGame && capturing {
				snap := l.tracker.Snapshot()
				slog.Info("game ended", "placement", snap.Placement)
				l.store.FinalizeGame(snap.Placement)
				l.store.Close()
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
					slog.Error("screenshot failed", "err", err)
					continue
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
