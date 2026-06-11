package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

// startTestServer starts a Server on an ephemeral port and returns it, its
// bound address, the daemon-style cancel func, and the Serve error channel.
func startTestServer(t *testing.T, events chan parser.GameEvent) (*Server, string, context.CancelFunc, chan error) {
	t.Helper()

	srv := New(gamestate.New(), nil, events)
	ctx, cancel := context.WithCancel(context.Background())

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ctx, "127.0.0.1:0")
	}()

	deadline := time.Now().Add(5 * time.Second)
	for srv.BoundAddr() == "" {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("server did not bind within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return srv, srv.BoundAddr(), cancel, serveErr
}

type eventStream interface {
	Recv() (*bspb.GameEvent, error)
}

// waitForStreamAttached feeds events into the fan-out until the stream
// delivers one, proving the server-side handler has subscribed and the
// stream is live. (A single send can race the handler's subscribe call:
// fanOut may broadcast before the subscriber list contains the stream.)
func waitForStreamAttached(t *testing.T, stream eventStream, events chan<- parser.GameEvent) {
	t.Helper()

	recvDone := make(chan error, 1)
	go func() {
		_, err := stream.Recv()
		recvDone <- err
	}()

	feedStop := make(chan struct{})
	defer close(feedStop)
	go func() {
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-feedStop:
				return
			case <-tick.C:
				select {
				case events <- parser.GameEvent{Type: parser.EventTagChange, Timestamp: time.Now()}:
				default:
				}
			}
		}
	}()

	select {
	case err := <-recvDone:
		if err != nil {
			t.Fatalf("receiving event on live stream: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream never received an event; handler not attached")
	}
}

// drainUntilError consumes any buffered events on the stream and returns the
// terminating error, or fails the test if the stream keeps delivering.
func drainUntilError(t *testing.T, stream eventStream) {
	t.Helper()

	errCh := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				errCh <- err
				return
			}
		}
	}()
	select {
	case <-errCh:
		// Terminated as expected.
	case <-time.After(3 * time.Second):
		t.Fatal("stream still open after Stop; expected termination")
	}
}

// TestStopTerminatesActiveStream is the regression test for audit M11 bug 2:
// GracefulStop never completed while a StreamGameEvents client (e.g. a TUI)
// was attached, because fanOut exiting on ctx.Done did not close subscriber
// channels. Pre-fix the equivalent shutdown path hangs forever; post-fix Stop
// must return within the bounded timeout.
func TestStopTerminatesActiveStream(t *testing.T) {
	events := make(chan parser.GameEvent, 8)
	srv, addr, cancel, serveErr := startTestServer(t, events)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}
	defer conn.Close()
	client := bspb.NewBattlestreamServiceClient(conn)

	stream, err := client.StreamGameEvents(context.Background(), &bspb.StreamRequest{})
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	waitForStreamAttached(t, stream, events)

	// Daemon shutdown sequence: cancel the context (stops fanOut), then Stop.
	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()

	stopDone := make(chan struct{})
	go func() {
		srv.Stop(stopCtx)
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(6 * time.Second):
		t.Fatal("Stop did not complete with an active StreamGameEvents client attached")
	}

	// Serve must have fully returned.
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Stop")
	}

	// The stream must be terminated (EOF or transport error) after any
	// buffered events drain.
	drainUntilError(t, stream)
}

// TestStopIsGracefulWhenStreamsClose verifies Stop drains via GracefulStop
// (not the hard-stop fallback) once subscriber channels are closed: with a
// generous deadline, Stop must return well before the deadline because
// closing the subscriber channel ends the streaming RPC.
func TestStopIsGracefulWhenStreamsClose(t *testing.T) {
	events := make(chan parser.GameEvent, 8)
	srv, addr, cancel, _ := startTestServer(t, events)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}
	defer conn.Close()
	client := bspb.NewBattlestreamServiceClient(conn)

	stream, err := client.StreamGameEvents(context.Background(), &bspb.StreamRequest{})
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	waitForStreamAttached(t, stream, events)

	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()

	start := time.Now()
	srv.Stop(stopCtx)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Stop took %v; expected prompt graceful drain, not hard-stop fallback", elapsed)
	}
}

// TestStopWithoutServe verifies Stop is a safe no-op when Serve never ran.
func TestStopWithoutServe(t *testing.T) {
	srv := New(gamestate.New(), nil, make(chan parser.GameEvent))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	srv.Stop(ctx) // must not panic or block
}

// TestSubscribeAfterStopReturnsClosedChannel verifies a late subscriber gets
// a closed channel (its stream loop ends immediately) instead of one that
// would never be closed.
func TestSubscribeAfterStopReturnsClosedChannel(t *testing.T) {
	srv := New(gamestate.New(), nil, make(chan parser.GameEvent))
	srv.closeAllSubscribers()

	ch := srv.Subscribe()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel, got an event")
		}
	default:
		t.Fatal("expected closed channel, got open channel")
	}

	// Unsubscribing the already-closed channel must not double-close.
	srv.Unsubscribe(ch)
}
