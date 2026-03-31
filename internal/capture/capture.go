// Package capture provides screen capture and game-state correlation for bscapture.
package capture

import (
	"context"
	"image"

	"battlestream.fixates.io/internal/parser"
)

// EventSource produces parsed game events from Power.log.
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
