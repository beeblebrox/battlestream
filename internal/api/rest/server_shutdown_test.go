package rest

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	grpcserver "battlestream.fixates.io/internal/api/grpc"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

// startTestServer starts a REST server on an ephemeral port and returns it,
// its base URL, and the context cancel func.
func startTestServer(t *testing.T) (*Server, string, context.CancelFunc, chan error) {
	t.Helper()

	g := grpcserver.New(gamestate.New(), nil, make(chan parser.GameEvent))
	s := New(g, "")
	ctx, cancel := context.WithCancel(context.Background())

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.Serve(ctx, "127.0.0.1:0")
	}()

	deadline := time.Now().Add(5 * time.Second)
	for s.BoundAddr() == "" {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("server did not bind within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return s, "http://" + s.BoundAddr(), cancel, serveErr
}

// TestShutdownSynchronousAndOrdered is the regression test for audit M11
// bug 1: shutdown was fired in an unawaited goroutine, so the caller could
// close the store while requests were still in flight. Shutdown must now
// block until the server is fully drained, after which new requests fail.
func TestShutdownSynchronousAndOrdered(t *testing.T) {
	s, base, cancel, serveErr := startTestServer(t)
	defer cancel()

	// Server is up: a normal request succeeds.
	resp, err := http.Get(base + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/health = %d, want 200", resp.StatusCode)
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := s.Shutdown(shutCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Shutdown returned synchronously — Serve must already have exited.
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Shutdown")
	}

	// Further requests must fail: the listener is closed.
	if resp, err := http.Get(base + "/v1/health"); err == nil {
		resp.Body.Close()
		t.Fatal("expected request after Shutdown to fail, got a response")
	}
}

// TestShutdownDrainsInflightSSE verifies that a long-lived in-flight request
// (an SSE stream, which only ends on client disconnect) is completed
// gracefully — not killed — and that Shutdown still returns promptly instead
// of waiting out the whole grace period.
func TestShutdownDrainsInflightSSE(t *testing.T) {
	s, base, cancel, _ := startTestServer(t)
	defer cancel()

	type sseResult struct {
		status int
		err    error
	}
	sseDone := make(chan sseResult, 1)
	go func() {
		// Blocks until the handler returns: with no events flowing, the SSE
		// handler never flushes, so response headers only arrive when the
		// handler exits.
		resp, err := http.Get(base + "/v1/events")
		if err != nil {
			sseDone <- sseResult{err: err}
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		sseDone <- sseResult{status: resp.StatusCode}
	}()

	// Give the request time to reach the handler and park in its select.
	time.Sleep(300 * time.Millisecond)

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	start := time.Now()
	if err := s.Shutdown(shutCtx); err != nil {
		t.Fatalf("Shutdown with in-flight SSE stream: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Shutdown took %v; expected prompt drain of the SSE handler", elapsed)
	}

	// The in-flight request must have completed normally (drained, not killed).
	select {
	case res := <-sseDone:
		if res.err != nil {
			t.Fatalf("in-flight SSE request was killed instead of drained: %v", res.err)
		}
		if res.status != http.StatusOK {
			t.Fatalf("in-flight SSE request status = %d, want 200", res.status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight SSE request never completed after Shutdown")
	}
}

// TestShutdownWithoutServe verifies Shutdown is a safe no-op when Serve never ran.
func TestShutdownWithoutServe(t *testing.T) {
	g := grpcserver.New(gamestate.New(), nil, make(chan parser.GameEvent))
	s := New(g, "")
	ctx, cancelCtx := context.WithTimeout(context.Background(), time.Second)
	defer cancelCtx()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown without Serve: %v", err)
	}
}
