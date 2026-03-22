// Package rest implements the REST, WebSocket, and SSE API server.
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	grpcserver "battlestream.fixates.io/internal/api/grpc"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/stats"
	"battlestream.fixates.io/internal/store"
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
	mux.HandleFunc("GET /v1/game/{game_id}/turns", s.withAuth(s.handleGetTurnSnapshots))
	mux.HandleFunc("GET /v1/player/{name}", s.withAuth(s.handleGetPlayer))
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// WebSocket hub
	mux.HandleFunc("GET /ws/events", s.withAuth(s.handleWebSocket))

	// SSE endpoint
	mux.HandleFunc("GET /v1/events", s.withAuth(s.handleSSE))

	// Dashboard — serve embedded SPA
	dashSub, _ := fs.Sub(dashboardFS, "dashboard")
	mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(dashSub))))
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
	})

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
	mode := r.URL.Query().Get("mode")
	metas, err := s.grpc.GetStore().ListGames(0, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	filtered := filterMetasByMode(metas, mode)
	results := make([]stats.GameResult, len(filtered))
	for i, m := range filtered {
		results[i] = stats.GameResult{
			Placement: m.Placement,
			EndTime:   time.Unix(m.EndTime, 0),
			IsDuos:    m.IsDuos,
		}
	}
	s.writeJSON(w, stats.Compute(results))
}

func (s *Server) handleListGames(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	allGames, err := s.grpc.GetStore().ListGames(0, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	filtered := filterMetasByMode(allGames, mode)
	total := len(filtered)

	// Apply pagination.
	if offset > len(filtered) {
		filtered = nil
	} else {
		filtered = filtered[offset:]
		if limit > 0 && limit < len(filtered) {
			filtered = filtered[:limit]
		}
	}
	if filtered == nil {
		filtered = []store.GameMeta{}
	}

	s.writeJSON(w, struct {
		Games []store.GameMeta `json:"games"`
		Total int              `json:"total"`
	}{Games: filtered, Total: total})
}

func (s *Server) handleGetTurnSnapshots(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("game_id")
	if id == "" {
		http.Error(w, "game_id required", http.StatusBadRequest)
		return
	}
	snapshots, err := s.grpc.GetStore().GetTurnSnapshots(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if snapshots == nil {
		snapshots = []gamestate.TurnSnapshot{}
	}
	s.writeJSON(w, snapshots)
}

// filterMetasByMode filters game metas by mode: "solo", "duos", or "all"/empty.
func filterMetasByMode(metas []store.GameMeta, mode string) []store.GameMeta {
	switch mode {
	case "solo":
		out := make([]store.GameMeta, 0, len(metas))
		for _, m := range metas {
			if !m.IsDuos {
				out = append(out, m)
			}
		}
		return out
	case "duos":
		out := make([]store.GameMeta, 0, len(metas))
		for _, m := range metas {
			if m.IsDuos {
				out = append(out, m)
			}
		}
		return out
	default:
		return metas
	}
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
		winThreshold := 4
		if m.IsDuos {
			winThreshold = 2
		}
		if m.Placement <= winThreshold {
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
	Opponent        *gamestate.PlayerState       `json:"opponent,omitempty"`
	Board           []gamestate.MinionState      `json:"board"`
	OpponentBoard   []gamestate.MinionState      `json:"opponent_board,omitempty"`
	Modifications   []gamestate.StatMod          `json:"modifications"`
	BuffSources     []gamestate.BuffSource       `json:"buff_sources,omitempty"`
	AbilityCounters []gamestate.AbilityCounter   `json:"ability_counters,omitempty"`
	Enchantments    []gamestate.Enchantment      `json:"enchantments,omitempty"`
	AvailableTribes []string                     `json:"available_tribes,omitempty"`
	AnomalyCardID      string                    `json:"anomaly_card_id,omitempty"`
	AnomalyName        string                    `json:"anomaly_name,omitempty"`
	AnomalyDescription string                    `json:"anomaly_description,omitempty"`
	StartTimeUnix   int64                        `json:"start_time_unix"`
	EndTimeUnix     int64                        `json:"end_time_unix,omitempty"`
	Placement       int                          `json:"placement"`
	IsDuos                 bool                       `json:"is_duos,omitempty"`
	Partner                *gamestate.PlayerState     `json:"partner,omitempty"`
	PartnerBoard           []gamestate.MinionState    `json:"partner_board,omitempty"`
	PartnerBoardTurn       int                        `json:"partner_board_turn,omitempty"`
	PartnerBoardStale      bool                       `json:"partner_board_stale"`
	PartnerBuffSources     []gamestate.BuffSource     `json:"partner_buff_sources,omitempty"`
	PartnerAbilityCounters []gamestate.AbilityCounter `json:"partner_ability_counters,omitempty"`
}

func gameStateToJSON(s gamestate.BGGameState) gameStateJSON {
	j := gameStateJSON{
		GameID:             s.GameID,
		Phase:              string(s.Phase),
		Turn:               s.Turn,
		TavernTier:         s.TavernTier,
		Player:             s.Player,
		Opponent:           s.Opponent,
		Board:              s.Board,
		OpponentBoard:      s.OpponentBoard,
		Modifications:      s.Modifications,
		BuffSources:        s.BuffSources,
		AbilityCounters:    s.AbilityCounters,
		Enchantments:       s.Enchantments,
		AvailableTribes:    s.AvailableTribes,
		AnomalyCardID:      s.AnomalyCardID,
		AnomalyName:        s.AnomalyName,
		AnomalyDescription: s.AnomalyDescription,
		StartTimeUnix:      s.StartTime.Unix(),
		Placement:          s.Placement,
		IsDuos:                 s.IsDuos,
		Partner:                s.Partner,
		PartnerBuffSources:     s.PartnerBuffSources,
		PartnerAbilityCounters: s.PartnerAbilityCounters,
	}
	if s.EndTime != nil {
		j.EndTimeUnix = s.EndTime.Unix()
	}
	if s.PartnerBoard != nil {
		j.PartnerBoard = s.PartnerBoard.Minions
		j.PartnerBoardTurn = s.PartnerBoard.Turn
		j.PartnerBoardStale = s.PartnerBoard.Stale
	}
	return j
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
