// Package rest implements the REST, WebSocket, and SSE API server.
package rest

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	grpcserver "battlestream.fixates.io/internal/api/grpc"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/stats"
	"battlestream.fixates.io/internal/store"
)

// Browser-facing requests (WebSocket upgrades, SSE) are only accepted from
// loopback origins; requests without an Origin header (curl, native clients)
// are always allowed. This prevents arbitrary websites from reading the
// event feeds of a tracker bound to localhost.
var upgrader = websocket.Upgrader{
	CheckOrigin: isAllowedOrigin,
}

// isLoopbackHostname reports whether host (no port) is a localhost form:
// "localhost" or a loopback IP literal (127.0.0.0/8, ::1).
func isLoopbackHostname(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// hostWithoutPort strips an optional :port from a Host header value.
func hostWithoutPort(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}

// isAllowedOrigin allows requests with no Origin header (non-browser
// clients) and browser requests originating from a loopback host (any
// port, any scheme). Everything else is rejected.
func isAllowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLoopbackHostname(u.Hostname())
}

// isAllowedRequestHost reports whether the request Host header (port already
// stripped) is acceptable: localhost forms are always allowed, plus the host
// the server was configured to bind. A wildcard bind (empty host, 0.0.0.0,
// or ::) means the operator explicitly exposed the server beyond loopback,
// so any Host is accepted in that case.
func isAllowedRequestHost(host, bindHost string) bool {
	if isLoopbackHostname(host) {
		return true
	}
	switch bindHost {
	case "", "0.0.0.0", "::":
		return true
	}
	return strings.EqualFold(host, bindHost)
}

// hostCheckHandler rejects requests whose Host header matches neither a
// localhost form nor the configured bind host. This defeats DNS rebinding
// attacks, where a malicious domain resolves to 127.0.0.1 and the browser
// sends the attacker's domain in the Host header.
func hostCheckHandler(bindHost string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowedRequestHost(hostWithoutPort(r.Host), bindHost) {
			http.Error(w, "forbidden: unrecognized Host header", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Server is the REST + WebSocket + SSE HTTP server.
type Server struct {
	grpc   *grpcserver.Server
	apiKey string

	hub *wsHub

	mu      sync.Mutex // guards httpSrv and lis
	httpSrv *http.Server
	lis     net.Listener

	shutdownCh   chan struct{} // closed when Shutdown begins; wakes SSE handlers
	shutdownOnce sync.Once
}

// New creates a REST Server.
func New(grpc *grpcserver.Server, apiKey string) *Server {
	return &Server{
		grpc:       grpc,
		apiKey:     apiKey,
		hub:        newHub(),
		shutdownCh: make(chan struct{}),
	}
}

// Serve starts the HTTP server on addr and blocks until the server stops
// accepting connections. Shutdown is initiated by calling Shutdown; ctx only
// governs the WS hub and event-subscription goroutines. Callers must call
// Shutdown to terminate the server and drain in-flight requests.
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
	mux.HandleFunc("GET /v1/cardnames", s.withAuth(s.handleGetCardNames))
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// WebSocket hub
	mux.HandleFunc("GET /ws/events", s.withAuth(s.handleWebSocket))

	// SSE endpoint
	mux.HandleFunc("GET /v1/events", s.withAuth(s.handleSSE))

	// Dashboard — serve embedded SPA
	if dashSub, err := fs.Sub(dashboardFS, "dashboard"); err != nil {
		// Should be impossible with a correct embed, but don't serve a
		// broken FileServer that panics later — fail with an explicit 404.
		slog.Error("dashboard assets unavailable", "err", err)
		mux.HandleFunc("GET /dashboard/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "dashboard assets unavailable", http.StatusNotFound)
		})
	} else {
		mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(dashSub))))
	}
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
	})

	// Root redirect to dashboard
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	})

	// Host derived from the configured bind address; used by the Host-header
	// check (DNS rebinding protection).
	bindHost := ""
	if h, _, err := net.SplitHostPort(addr); err == nil {
		bindHost = h
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("rest listen on %s: %w", addr, err)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           hostCheckHandler(bindHost, mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.mu.Lock()
	s.httpSrv = srv
	s.lis = lis
	s.mu.Unlock()

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
					select {
					case s.hub.broadcast <- data:
					case <-s.hub.done:
						return
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	slog.Info("REST server listening", "addr", addr)

	if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("rest serve: %w", err)
	}
	return nil
}

// BoundAddr returns the address the server is actually listening on
// (useful when Serve was given a ":0" port), or "" before Serve has bound.
func (s *Server) BoundAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lis == nil {
		return ""
	}
	return s.lis.Addr().String()
}

// Shutdown stops accepting new connections and blocks until in-flight
// requests have drained or ctx expires; on expiry, remaining connections are
// forcibly closed. Long-lived SSE handlers are signalled to finish so the
// drain can complete promptly. Safe to call when Serve was never started
// (returns nil).
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()
	if srv == nil {
		return nil
	}

	// Wake SSE handlers; otherwise srv.Shutdown waits the full grace period
	// for streams that only end on client disconnect.
	s.shutdownOnce.Do(func() { close(s.shutdownCh) })

	if err := srv.Shutdown(ctx); err != nil {
		// Grace period expired — hard-close whatever is left.
		_ = srv.Close()
		return fmt.Errorf("rest shutdown: %w", err)
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
	filtered := filterMetasByMode(filterCompleteGames(metas), mode)
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
	filtered := filterMetasByMode(filterCompleteGames(allGames), mode)
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

// filterCompleteGames excludes stale/incomplete games (placement=0) from
// results. Delegates to the shared store helper so REST and gRPC stay in sync.
func filterCompleteGames(metas []store.GameMeta) []store.GameMeta {
	return store.FilterCompleteGames(metas)
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

func (s *Server) handleGetCardNames(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, gamestate.CardNames())
}

func (s *Server) handleGetGame(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("game_id")
	if id == "" {
		http.Error(w, "game_id required", http.StatusBadRequest)
		return
	}
	gs, err := s.grpc.GetStore().GetGame(id)
	if err != nil {
		if errors.Is(err, store.ErrGameNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
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
		if errors.Is(err, store.ErrGameNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
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
	// The store does not record player names per game, so per-player
	// filtering is impossible. The endpoint previously returned global stats
	// for any name, which was misleading. Use /v1/stats/aggregate instead.
	http.Error(w,
		"per-player stats are not implemented: the store does not record player names per game; use /v1/stats/aggregate",
		http.StatusNotImplemented)
}

// --- WebSocket handler ---

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "err", err)
		return
	}
	client := &wsClient{hub: s.hub, conn: conn, send: make(chan []byte, 64)}
	select {
	case s.hub.register <- client:
	case <-s.hub.done:
		// Hub already stopped (server shutting down) — drop the connection
		// instead of blocking forever on the register channel.
		conn.Close()
		return
	}

	go client.writePump()
	go client.readPump()
}

// --- SSE handler ---

// sseKeepaliveInterval is how often the SSE handler emits a comment line so
// EventSource clients and proxies can detect dead connections. Var so tests
// can shorten it.
var sseKeepaliveInterval = 25 * time.Second

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Same browser-origin policy as the WebSocket endpoint: no Origin header
	// (curl, native clients) is fine; browser origins must be loopback.
	if !isAllowedOrigin(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Echo the (already validated, loopback) request Origin instead of "*";
	// omit the header entirely for non-browser clients.
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Flush headers immediately so EventSource clients leave the CONNECTING
	// state without waiting for the first event.
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	eventCh := s.grpc.Subscribe()
	defer s.grpc.Unsubscribe(eventCh)

	keepalive := time.NewTicker(sseKeepaliveInterval)
	defer keepalive.Stop()

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
		case <-keepalive.C:
			// Comment line: ignored by EventSource, keeps the connection
			// alive and surfaces dead peers via the write error path.
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-s.shutdownCh:
			return
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
	expected := []byte("Bearer " + s.apiKey)
	return func(w http.ResponseWriter, r *http.Request) {
		auth := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(auth, expected) != 1 {
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
	done       chan struct{} // closed when run() exits; senders select on it
	mu         sync.Mutex
}

const (
	wsWriteTimeout = 10 * time.Second
	wsPingInterval = 30 * time.Second
	wsPongTimeout  = 60 * time.Second
)

func newHub() *wsHub {
	return &wsHub{
		clients:    make(map[*wsClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *wsClient),
		unregister: make(chan *wsClient, 8),
		done:       make(chan struct{}),
	}
}

func (h *wsHub) run(ctx context.Context) {
	defer close(h.done)
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
	ticker := time.NewTicker(wsPingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if !ok {
				// Hub closed the channel — send close frame and exit.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *wsClient) readPump() {
	defer func() {
		select {
		case c.hub.unregister <- c:
		case <-c.hub.done:
			// Hub stopped — nobody is draining unregister; don't block.
		}
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
