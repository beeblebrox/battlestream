package tui

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
)

// Client wraps the gRPC connection to the battlestream daemon.
type Client struct {
	conn *grpc.ClientConn
	svc  bspb.BattlestreamServiceClient
}

// Dial connects to the daemon at addr. Caller must call Close() when done.
func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dialing daemon at %s: %w", addr, err)
	}
	return &Client{conn: conn, svc: bspb.NewBattlestreamServiceClient(conn)}, nil
}

// Close tears down the gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetCurrentGame fetches a full game state snapshot.
func (c *Client) GetCurrentGame(ctx context.Context) (*bspb.GameState, error) {
	return c.svc.GetCurrentGame(ctx, &bspb.GetCurrentGameRequest{})
}

// GetAggregate fetches aggregate stats.
func (c *Client) GetAggregate(ctx context.Context) (*bspb.AggregateStats, error) {
	return c.svc.GetAggregate(ctx, &bspb.GetAggregateRequest{})
}

// StreamEvents opens a server-side stream and returns a channel that receives
// events until ctx is cancelled or the stream closes.
func (c *Client) StreamEvents(ctx context.Context) (<-chan *bspb.GameEvent, error) {
	stream, err := c.svc.StreamGameEvents(ctx, &bspb.StreamRequest{})
	if err != nil {
		return nil, fmt.Errorf("opening event stream: %w", err)
	}

	ch := make(chan *bspb.GameEvent, 64)
	go func() {
		defer close(ch)
		for {
			e, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case ch <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
