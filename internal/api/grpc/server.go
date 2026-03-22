// Package grpc implements the BattlestreamService gRPC server.
package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
	"battlestream.fixates.io/internal/store"
)

// Server implements BattlestreamServiceServer and holds shared dependencies.
type Server struct {
	bspb.UnimplementedBattlestreamServiceServer

	gs     *gamestate.Machine
	st     *store.Store
	events <-chan parser.GameEvent

	subsMu sync.Mutex
	subs   []chan parser.GameEvent

	grpcSrv *grpc.Server
}

// New creates a new gRPC Server.
func New(gs *gamestate.Machine, st *store.Store, events <-chan parser.GameEvent) *Server {
	return &Server{
		gs:     gs,
		st:     st,
		events: events,
	}
}

// Serve starts the gRPC server on addr. Blocks until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen on %s: %w", addr, err)
	}

	s.grpcSrv = grpc.NewServer()
	bspb.RegisterBattlestreamServiceServer(s.grpcSrv, s)
	reflection.Register(s.grpcSrv)

	slog.Info("gRPC server listening", "addr", addr)

	go s.fanOut(ctx)

	go func() {
		<-ctx.Done()
		s.grpcSrv.GracefulStop()
	}()

	if err := s.grpcSrv.Serve(lis); err != nil && ctx.Err() == nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}

// --- BattlestreamServiceServer implementation ---

// GetCurrentGame returns the current in-memory game state snapshot.
func (s *Server) GetCurrentGame(_ context.Context, _ *bspb.GetCurrentGameRequest) (*bspb.GameState, error) {
	return gameStateToProto(s.gs.State()), nil
}

// GetGame retrieves a historical game by ID from the store.
func (s *Server) GetGame(_ context.Context, req *bspb.GetGameRequest) (*bspb.GameState, error) {
	if req.GameId == "" {
		return nil, status.Error(codes.InvalidArgument, "game_id is required")
	}
	gs, err := s.st.GetGame(req.GameId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "game %q not found: %v", req.GameId, err)
	}
	return gameStateToProto(*gs), nil
}

// StreamGameEvents streams live game events to the caller until the stream is closed.
func (s *Server) StreamGameEvents(_ *bspb.StreamRequest, stream grpc.ServerStreamingServer[bspb.GameEvent]) error {
	ch := s.subscribe()
	defer s.unsubscribe(ch)

	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(parserEventToProto(e)); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// GetAggregate returns aggregate stats across all recorded games.
func (s *Server) GetAggregate(_ context.Context, _ *bspb.GetAggregateRequest) (*bspb.AggregateStats, error) {
	agg, err := s.st.GetAggregate()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting aggregate: %v", err)
	}

	return &bspb.AggregateStats{
		GamesPlayed:    int32(agg.GamesPlayed),
		Wins:           int32(agg.Wins),
		Losses:         int32(agg.Losses),
		AvgPlacement:   agg.AvgPlacement,
		BestPlacement:  int32(agg.BestPlacement),
		WorstPlacement: int32(agg.WorstPlacement),
	}, nil
}

// ListGames returns paginated game history metadata.
func (s *Server) ListGames(_ context.Context, req *bspb.ListGamesRequest) (*bspb.ListGamesResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	metas, err := s.st.ListGames(limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listing games: %v", err)
	}

	resp := &bspb.ListGamesResponse{
		Total: int32(len(metas)),
	}
	for _, m := range metas {
		resp.Games = append(resp.Games, &bspb.GameMeta{
			GameId:        m.GameID,
			StartTimeUnix: m.StartTime,
			EndTimeUnix:   m.EndTime,
			Placement:     int32(m.Placement),
			IsDuos:        m.IsDuos,
		})
	}
	return resp, nil
}

// GetPlayerProfile returns per-player stats derived from stored game history.
func (s *Server) GetPlayerProfile(_ context.Context, req *bspb.GetPlayerRequest) (*bspb.PlayerProfile, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	metas, err := s.st.ListGames(0, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listing games: %v", err)
	}

	profile := &bspb.PlayerProfile{Name: req.Name}
	var totalPlacement int
	best := int32(8)

	for _, m := range metas {
		// Filter by player name stored in meta. For now include all — player
		// filtering will be enhanced once per-player keys are added to store.
		profile.GamesPlayed++
		profile.GameIds = append(profile.GameIds, m.GameID)
		totalPlacement += m.Placement
		winThreshold := 4
		if m.IsDuos {
			winThreshold = 2
		}
		if int32(m.Placement) <= int32(winThreshold) {
			profile.Wins++
		} else {
			profile.Losses++
		}
		if int32(m.Placement) < best {
			best = int32(m.Placement)
		}
	}

	if profile.GamesPlayed > 0 {
		profile.AvgPlacement = float64(totalPlacement) / float64(profile.GamesPlayed)
		profile.BestPlacement = best
	}

	return profile, nil
}

// --- Pub/sub for streaming ---

// Subscribe returns a channel that receives copies of game events.
func (s *Server) Subscribe() chan parser.GameEvent {
	return s.subscribe()
}

// Unsubscribe removes a subscriber channel.
func (s *Server) Unsubscribe(ch chan parser.GameEvent) {
	s.unsubscribe(ch)
}

func (s *Server) subscribe() chan parser.GameEvent {
	ch := make(chan parser.GameEvent, 512)
	s.subsMu.Lock()
	s.subs = append(s.subs, ch)
	s.subsMu.Unlock()
	return ch
}

func (s *Server) unsubscribe(ch chan parser.GameEvent) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	subs := s.subs[:0]
	for _, sub := range s.subs {
		if sub != ch {
			subs = append(subs, sub)
		}
	}
	s.subs = subs
	close(ch)
}

func (s *Server) fanOut(ctx context.Context) {
	var dropped atomic.Int64
	for {
		select {
		case e, ok := <-s.events:
			if !ok {
				return
			}
			s.subsMu.Lock()
			for _, ch := range s.subs {
				select {
				case ch <- e:
				default:
					n := dropped.Add(1)
					// Log first drop, then every 1000th to avoid flooding the log.
					if n == 1 || n%1000 == 0 {
						slog.Warn("dropping events for slow subscriber", "total_dropped", n)
					}
				}
			}
			s.subsMu.Unlock()
		case <-ctx.Done():
			if n := dropped.Load(); n > 0 {
				slog.Warn("subscriber event drop summary", "total_dropped", n)
			}
			return
		}
	}
}

// GetCurrentGameState is a convenience accessor for the REST layer.
func (s *Server) GetCurrentGameState() gamestate.BGGameState {
	return s.gs.State()
}

// GetStore exposes the store for REST handlers.
func (s *Server) GetStore() *store.Store {
	return s.st
}

// --- Conversion helpers ---

func gameStateToProto(s gamestate.BGGameState) *bspb.GameState {
	gs := &bspb.GameState{
		GameId:        s.GameID,
		Phase:         string(s.Phase),
		Turn:          int32(s.Turn),
		TavernTier:    int32(s.TavernTier),
		Placement:     int32(s.Placement),
		StartTimeUnix: s.StartTime.Unix(),
		Player:        playerStateToProto(s.Player),
	}
	if s.EndTime != nil {
		gs.EndTimeUnix = s.EndTime.Unix()
	}
	if s.Opponent != nil {
		gs.Opponent = playerStateToProto(*s.Opponent)
	}
	for _, m := range s.Board {
		gs.Board = append(gs.Board, minionToProto(m))
	}
	for _, m := range s.OpponentBoard {
		gs.OpponentBoard = append(gs.OpponentBoard, minionToProto(m))
	}
	for _, mod := range s.Modifications {
		gs.Modifications = append(gs.Modifications, statModToProto(mod))
	}
	for _, bs := range s.BuffSources {
		gs.BuffSources = append(gs.BuffSources, buffSourceToProto(bs))
	}
	for _, e := range s.Enchantments {
		gs.Enchantments = append(gs.Enchantments, enchantmentToProto(e))
	}
	for _, ac := range s.AbilityCounters {
		gs.AbilityCounters = append(gs.AbilityCounters, abilityCounterToProto(ac))
	}
	gs.AvailableTribes = s.AvailableTribes
	gs.AnomalyCardId = s.AnomalyCardID
	gs.AnomalyName = s.AnomalyName
	gs.AnomalyDescription = s.AnomalyDescription
	gs.IsDuos = s.IsDuos
	if s.Partner != nil {
		gs.Partner = playerStateToProto(*s.Partner)
	}
	if s.PartnerBoard != nil {
		for _, mn := range s.PartnerBoard.Minions {
			gs.PartnerBoard = append(gs.PartnerBoard, minionToProto(mn))
		}
		gs.PartnerBoardTurn = int32(s.PartnerBoard.Turn)
		gs.PartnerBoardStale = s.PartnerBoard.Stale
	}
	for _, bs := range s.PartnerBuffSources {
		gs.PartnerBuffSources = append(gs.PartnerBuffSources, buffSourceToProto(bs))
	}
	for _, ac := range s.PartnerAbilityCounters {
		gs.PartnerAbilityCounters = append(gs.PartnerAbilityCounters, abilityCounterToProto(ac))
	}
	return gs
}

func playerStateToProto(p gamestate.PlayerState) *bspb.PlayerStats {
	return &bspb.PlayerStats{
		Name:        p.Name,
		HeroCardId:  p.HeroCardID,
		Health:      int32(p.Health),
		MaxHealth:   int32(p.MaxHealth),
		Damage:      int32(p.Damage),
		Armor:       int32(p.Armor),
		SpellPower:  int32(p.SpellPower),
		TripleCount: int32(p.TripleCount),
		TavernTier:  int32(p.TavernTier),
		WinStreak:   int32(p.WinStreak),
		LossStreak:  int32(p.LossStreak),
		CurrentGold: int32(p.CurrentGold),
		MaxGold:     int32(p.MaxGold),
	}
}

func minionToProto(m gamestate.MinionState) *bspb.MinionState {
	pb := &bspb.MinionState{
		EntityId:   int32(m.EntityID),
		CardId:     m.CardID,
		Name:       m.Name,
		Attack:     int32(m.Attack),
		Health:     int32(m.Health),
		MinionType: m.MinionType,
		BuffAttack: int32(m.BuffAttack),
		BuffHealth: int32(m.BuffHealth),
	}
	for _, e := range m.Enchantments {
		pb.Enchantments = append(pb.Enchantments, enchantmentToProto(e))
	}
	return pb
}

func statModToProto(mod gamestate.StatMod) *bspb.StatMod {
	return &bspb.StatMod{
		Turn:     int32(mod.Turn),
		Target:   mod.Target,
		Stat:     mod.Stat,
		Delta:    int32(mod.Delta),
		Source:   mod.Source,
		Category: mod.Category,
		CardId:   mod.CardID,
	}
}

func buffSourceToProto(bs gamestate.BuffSource) *bspb.BuffSource {
	return &bspb.BuffSource{
		Category: bs.Category,
		Attack:   int32(bs.Attack),
		Health:   int32(bs.Health),
	}
}

func enchantmentToProto(e gamestate.Enchantment) *bspb.Enchantment {
	return &bspb.Enchantment{
		EntityId:     int32(e.EntityID),
		CardId:       e.CardID,
		SourceCardId: e.SourceCardID,
		SourceName:   e.SourceName,
		TargetId:     int32(e.TargetID),
		AttackBuff:   int32(e.AttackBuff),
		HealthBuff:   int32(e.HealthBuff),
		Category:     e.Category,
	}
}

func abilityCounterToProto(ac gamestate.AbilityCounter) *bspb.AbilityCounter {
	return &bspb.AbilityCounter{
		Category: ac.Category,
		Value:    int32(ac.Value),
		Display:  ac.Display,
	}
}

func parserEventToProto(e parser.GameEvent) *bspb.GameEvent {
	tags := make(map[string]string, len(e.Tags))
	for k, v := range e.Tags {
		tags[k] = v
	}
	return &bspb.GameEvent{
		Type:          string(e.Type),
		TimestampUnix: e.Timestamp.Unix(),
		EntityId:      int32(e.EntityID),
		Tags:          tags,
		EntityName:    e.EntityName,
		CardId:        e.CardID,
	}
}
