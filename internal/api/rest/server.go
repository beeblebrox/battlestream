// Package rest implements the REST, WebSocket, and SSE API server.
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	grpcserver "battlestream.fixates.io/internal/api/grpc"
	"battlestream.fixates.io/internal/gamestate"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // allow all origins (localhost usage)
}

// Server is the REST + WebSocket + SSE HTTP server.
type Server struct {
	grpc   *grpcserver.Server
	apiKey string

	hub *wsHub
}

// New creates a REST Server.
func New(grpc *grpcserver.Server, apiKey string) *Server {
	return &Server{
		grpc:   grpc,
		apiKey: apiKey,
		hub:    newHub(),
	}
}

// Serve starts the HTTP server on addr. Blocks until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	// REST endpoints
	mux.HandleFunc("GET /v1/game/current", s.withAuth(s.handleGetCurrentGame))
	mux.HandleFunc("GET /v1/game/{game_id}", s.withAuth(s.handleGetGame))
	mux.HandleFunc("GET /v1/stats/aggregate", s.withAuth(s.handleGetAggregate))
	mux.HandleFunc("GET /v1/stats/games", s.withAuth(s.handleListGames))
	mux.HandleFunc("GET /v1/stats/games/{game_id}/modifications", s.withAuth(s.handleGetModifications))
	mux.HandleFunc("GET /v1/player/{name}", s.withAuth(s.handleGetPlayer))
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// WebSocket hub
	mux.HandleFunc("GET /ws/events", s.withAuth(s.handleWebSocket))

	// SSE endpoint
	mux.HandleFunc("GET /v1/events", s.withAuth(s.handleSSE))

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start WS hub
	go s.hub.run(ctx)

	// Subscribe to game events and broadcast to WS/SSE clients
	eventCh := s.grpc.Subscribe()
	go func() {
		defer s.grpc.Unsubscribe(eventCh)
		for {
			select {
			case e, ok := <-eventCh:
				if !ok {
					return
				}
				data, err := json.Marshal(e)
				if err == nil {
					s.hub.broadcast <- data
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	slog.Info("REST server listening", "addr", addr)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			slog.Error("REST server shutdown", "err", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("rest serve: %w", err)
	}
	return nil
}

// --- REST handlers ---

func (s *Server) handleGetCurrentGame(w http.ResponseWriter, r *http.Request) {
	state := s.grpc.GetCurrentGameState()
	s.writeJSON(w, gameStateToJSON(state))
}

func (s *Server) handleGetAggregate(w http.ResponseWriter, r *http.Request) {
	agg, err := s.grpc.GetStore().GetAggregate()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, agg)
}

func (s *Server) handleListGames(w http.ResponseWriter, r *http.Request) {
	games, err := s.grpc.GetStore().ListGames(50, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, games)
}

func (s *Server) handleGetGame(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("game_id")
	if id == "" {
		http.Error(w, "game_id required", http.StatusBadRequest)
		return
	}
	gs, err := s.grpc.GetStore().GetGame(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.writeJSON(w, gameStateToJSON(*gs))
}

func (s *Server) handleGetModifications(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("game_id")
	if id == "" {
		http.Error(w, "game_id required", http.StatusBadRequest)
		return
	}
	gs, err := s.grpc.GetStore().GetGame(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	type modsResponse struct {
		GameID        string               `json:"game_id"`
		Modifications []gamestate.StatMod  `json:"modifications"`
	}
	mods := gs.Modifications
	if mods == nil {
		mods = []gamestate.StatMod{}
	}
	s.writeJSON(w, modsResponse{GameID: id, Modifications: mods})
}

func (s *Server) handleGetPlayer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	metas, err := s.grpc.GetStore().ListGames(0, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	type playerProfile struct {
		Name          string   `json:"name"`
		GamesPlayed   int      `json:"games_played"`
		Wins          int      `json:"wins"`
		Losses        int      `json:"losses"`
		AvgPlacement  float64  `json:"avg_placement"`
		BestPlacement int      `json:"best_placement,omitempty"`
		GameIDs       []string `json:"game_ids"`
	}
	p := playerProfile{Name: name, BestPlacement: 8}
	var total int
	for _, m := range metas {
		p.GamesPlayed++
		p.GameIDs = append(p.GameIDs, m.GameID)
		total += m.Placement
		if m.Placement <= 4 {
			p.Wins++
		} else {
			p.Losses++
		}
		if m.Placement < p.BestPlacement {
			p.BestPlacement = m.Placement
		}
	}
	if p.GamesPlayed > 0 {
		p.AvgPlacement = float64(total) / float64(p.GamesPlayed)
	} else {
		p.BestPlacement = 0
	}
	s.writeJSON(w, p)
}

// --- WebSocket handler ---

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "err", err)
		return
	}
	client := &wsClient{hub: s.hub, conn: conn, send: make(chan []byte, 64)}
	s.hub.register <- client

	go client.writePump()
	go client.readPump()
}

// --- SSE handler ---

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	eventCh := s.grpc.Subscribe()
	defer s.grpc.Unsubscribe(eventCh)

	for {
		select {
		case e, ok := <-eventCh:
			if !ok {
				return
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// --- Auth middleware ---

func (s *Server) withAuth(h http.HandlerFunc) http.HandlerFunc {
	if s.apiKey == "" {
		return h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+s.apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("json encode", "err", err)
	}
}

// --- JSON DTOs ---

type gameStateJSON struct {
	GameID          string                       `json:"game_id"`
	Phase           string                       `json:"phase"`
	Turn            int                          `json:"turn"`
	TavernTier      int                          `json:"tavern_tier"`
	Player          gamestate.PlayerState        `json:"player"`
	Board           []gamestate.MinionState      `json:"board"`
	Modifications   []gamestate.StatMod          `json:"modifications"`
	BuffSources     []gamestate.BuffSource       `json:"buff_sources,omitempty"`
	AbilityCounters []gamestate.AbilityCounter   `json:"ability_counters,omitempty"`
	Enchantments    []gamestate.Enchantment      `json:"enchantments,omitempty"`
	StartTimeUnix   int64                        `json:"start_time_unix"`
	Placement       int                          `json:"placement"`
	IsDuos               bool                        `json:"is_duos,omitempty"`
	Partner              *gamestate.PlayerState       `json:"partner,omitempty"`
	PartnerBoard         []gamestate.MinionState      `json:"partner_board,omitempty"`
	PartnerBuffSources   []gamestate.BuffSource       `json:"partner_buff_sources,omitempty"`
	PartnerAbilityCounters []gamestate.AbilityCounter  `json:"partner_ability_counters,omitempty"`
}

func gameStateToJSON(s gamestate.BGGameState) gameStateJSON {
	return gameStateJSON{
		GameID:                 s.GameID,
		Phase:                  string(s.Phase),
		Turn:                   s.Turn,
		TavernTier:             s.TavernTier,
		Player:                 s.Player,
		Board:                  s.Board,
		Modifications:          s.Modifications,
		BuffSources:            s.BuffSources,
		AbilityCounters:        s.AbilityCounters,
		Enchantments:           s.Enchantments,
		StartTimeUnix:          s.StartTime.Unix(),
		Placement:              s.Placement,
		IsDuos:                 s.IsDuos,
		Partner:                s.Partner,
		PartnerBoard:           s.PartnerBoard,
		PartnerBuffSources:     s.PartnerBuffSources,
		PartnerAbilityCounters: s.PartnerAbilityCounters,
	}
}

// --- WebSocket hub ---

type wsHub struct {
	clients    map[*wsClient]bool
	broadcast  chan []byte
	register   chan *wsClient
	unregister chan *wsClient
	mu         sync.Mutex
}

func newHub() *wsHub {
	return &wsHub{
		clients:    make(map[*wsClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *wsClient),
		unregister: make(chan *wsClient),
	}
}

func (h *wsHub) run(ctx context.Context) {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case msg := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

type wsClient struct {
	hub  *wsHub
	conn *websocket.Conn
	send chan []byte
}

func (c *wsClient) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *wsClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
