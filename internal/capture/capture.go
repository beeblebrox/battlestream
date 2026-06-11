// Package capture provides screen capture and game-state correlation for bscapture.
package capture

import (
	"context"
	"image"

	"battlestream.fixates.io/internal/parser"
)

// EventCatchupComplete is a synthetic event emitted by an EventSource once it
// has finished replaying pre-existing log content and reached the live tail.
// The capture loop will not start a capture session before it has seen this
// marker: a backlog replay may pass through games that finished long ago, and
// starting a session mid-replay would attribute live screenshots to a
// historical game ID. The marker must be emitted in-order on the Events
// channel, after every event parsed from backlog content.
const EventCatchupComplete parser.EventType = "CAPTURE_CATCHUP_COMPLETE"

// EventSource produces parsed game events from Power.log.
// Implementations that replay historical log content (ReadFromStart) MUST
// emit an EventCatchupComplete event once the backlog is drained; the capture
// loop defers all session starts until then.
type EventSource interface {
	Events() <-chan parser.GameEvent
	Close() error
}

// StateTracker maintains game state from events.
// Apply must be called from a single goroutine.
// Snapshot and InGame are safe for concurrent use.
type StateTracker interface {
	Apply(event parser.GameEvent)
	Snapshot() CaptureState
	InGame() bool
}

// Screenshotter captures the display.
type Screenshotter interface {
	Capture(ctx context.Context) (image.Image, error)
}

// FrameStore persists frames and metadata.
type FrameStore interface {
	InitGame(gameID string) error
	SaveFrame(frame Frame) error
	FinalizeGame(placement int) error
	Close() error
}

// Frame bundles a captured image with its metadata.
type Frame struct {
	Sequence       int
	Image          image.Image
	State          CaptureState
	CaptureLatency int64 // milliseconds
}
