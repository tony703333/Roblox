package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"im/internal/auth"
	"im/internal/simplews"
	"im/internal/storage"
	"im/internal/ws"
)

// Server wraps the HTTP handlers for the IM customer service system.
type Server struct {
	hub        *ws.Hub
	auth       *auth.Manager
	settings   storage.AgencySettingsStore
	upgrader   simplews.Upgrader
	staticRoot string
}

func New(hub *ws.Hub, authManager *auth.Manager, settings storage.AgencySettingsStore, staticRoot string) *Server {
	return &Server{
		hub:      hub,
		auth:     authManager,
		settings: settings,
		upgrader: simplews.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		staticRoot: staticRoot,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/profile", s.handleProfile)
	mux.HandleFunc("/api/auth/register", s.handleRegister)
	mux.HandleFunc("/api/agents/online", s.handleOnlineAgents)
	mux.HandleFunc("/api/agencies/settings", s.handleAgencySettingsCollection)
	mux.HandleFunc("/api/agencies/settings/", s.handleAgencySettings)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/rooms", s.handleRooms)
	mux.HandleFunc("/api/rooms/", s.handleRoom)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	adminFS := http.FileServer(http.Dir(filepath.Join(s.staticRoot, "admin")))
	mux.Handle("/admin/", http.StripPrefix("/admin/", adminFS))

	clientFS := http.FileServer(http.Dir(filepath.Join(s.staticRoot, "client")))
	mux.Handle("/client/", http.StripPrefix("/client/", clientFS))

	staticFS := http.FileServer(http.Dir(filepath.Join(s.staticRoot, "static")))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusFound)
	})

	return s.withJSONHeaders(mux)
}

func (s *Server) readToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	return ""
}

func (s *Server) currentAccount(r *http.Request) (*auth.Account, error) {
	if s.auth == nil {
		return nil, auth.ErrUnauthorized
	}
	token := s.readToken(r)
	if token == "" {
		return nil, auth.ErrUnauthorized
	}
	return s.auth.Authenticate(r.Context(), token)
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (*auth.Account, bool) {
	account, err := s.currentAccount(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	if account.Role != auth.RoleAdmin {
		s.writeError(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	return account, true
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	token, account, err := s.auth.Login(r.Context(), payload.Username, payload.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, map[string]any{
		"token":   token,
		"account": account,
	}, http.StatusOK)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := s.readToken(r)
	if token == "" {
		s.writeError(w, http.StatusBadRequest, "token required")
		return
	}
	s.auth.Logout(r.Context(), token)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	account, err := s.currentAccount(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	s.writeJSON(w, account, http.StatusOK)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Agency      string `json:"agency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	role := auth.RolePlayer
	if payload.Role != "" {
		switch strings.ToLower(payload.Role) {
		case string(auth.RoleAdmin):
			role = auth.RoleAdmin
		case string(auth.RolePlayer):
			role = auth.RolePlayer
		default:
			s.writeError(w, http.StatusBadRequest, "unknown role")
			return
		}
	}

	creator := ""
	if tokenAccount, err := s.currentAccount(r); err == nil {
		creator = tokenAccount.Username
	}

	account, err := s.auth.CreateAccount(r.Context(), creator, role, payload.Agency, payload.Username, payload.Password, payload.DisplayName)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrAccountExists):
			s.writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, auth.ErrInvalidUsername), errors.Is(err, auth.ErrInvalidPassword):
			s.writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, auth.ErrForbidden):
			s.writeError(w, http.StatusForbidden, err.Error())
		default:
			s.writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	s.writeJSON(w, account, http.StatusCreated)
}

func (s *Server) handleOnlineAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	agents := s.hub.OnlineAgents()
	s.writeJSON(w, agents, http.StatusOK)
}

func (s *Server) handleAgencySettingsCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.settings == nil {
		http.Error(w, "settings store not configured", http.StatusServiceUnavailable)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	items, err := s.settings.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, items, http.StatusOK)
}

func (s *Server) handleAgencySettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		http.Error(w, "settings store not configured", http.StatusServiceUnavailable)
		return
	}
	agency := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/agencies/settings/"), "/")
	if agency == "" {
		http.Error(w, "agency required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		settings, err := s.settings.Get(r.Context(), agency)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, settings, http.StatusOK)
	case http.MethodPost, http.MethodPut:
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		var payload struct {
			ChargeAPI     string `json:"chargeApi"`
			WithdrawAPI   string `json:"withdrawApi"`
			BetAPI        string `json:"betApi"`
			PlayerInfoAPI string `json:"playerInfoApi"`
		}
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		data := &storage.AgencyAPISettings{
			Agency:        agency,
			ChargeAPI:     strings.TrimSpace(payload.ChargeAPI),
			WithdrawAPI:   strings.TrimSpace(payload.WithdrawAPI),
			BetAPI:        strings.TrimSpace(payload.BetAPI),
			PlayerInfoAPI: strings.TrimSpace(payload.PlayerInfoAPI),
		}
		if err := s.settings.Upsert(r.Context(), data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		updated, err := s.settings.Get(r.Context(), agency)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				s.writeJSON(w, data, http.StatusOK)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, updated, http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	role := r.URL.Query().Get("role")
	if role == "" {
		role = ws.RolePlayer
	}
	if role != ws.RoleAgent && role != ws.RolePlayer {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	roomID := r.URL.Query().Get("roomId")
	if roomID == "" {
		http.Error(w, "roomId is required", http.StatusBadRequest)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	displayName := r.URL.Query().Get("name")
	if displayName == "" {
		displayName = fmt.Sprintf("%s-%s", role, id)
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	client := ws.NewClient(s.hub, conn, roomID, id, role, displayName)
	if _, err := s.hub.Register(client); err != nil {
		_ = conn.Close()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	go client.WritePump()
	client.ReadPump()
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		rooms := s.hub.Rooms()
		s.writeJSON(w, rooms, http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoom(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "room id missing", http.StatusBadRequest)
		return
	}

	roomID := parts[0]

	if len(parts) > 1 {
		switch parts[1] {
		case "assign":
			if r.Method == http.MethodPost {
				s.handleAssign(roomID, w, r)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		case "messages":
			s.handleRoomMessages(roomID, w, r)
			return
		default:
			http.Error(w, "unknown action", http.StatusNotFound)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		snapshot, err := s.hub.RoomSnapshot(roomID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.writeJSON(w, snapshot, http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAssign(roomID string, w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var payload struct {
		AgentID     string `json:"agentId"`
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if payload.AgentID == "" {
		http.Error(w, "agentId is required", http.StatusBadRequest)
		return
	}

	if payload.DisplayName == "" {
		payload.DisplayName = payload.AgentID
	}

	participant, err := s.hub.AssignAgent(roomID, payload.AgentID, payload.DisplayName)
	if err != nil {
		if errors.Is(err, ws.ErrRoomNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.writeJSON(w, participant, http.StatusOK)
}

func (s *Server) handleRoomMessages(roomID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	sinceParam := r.URL.Query().Get("since")
	var since int64
	if sinceParam != "" {
		value, err := strconv.ParseInt(sinceParam, 10, 64)
		if err != nil {
			http.Error(w, "invalid since parameter", http.StatusBadRequest)
			return
		}
		if value > 0 {
			since = value
		}
	}

	history, nextSeq, err := s.hub.MessagesSince(roomID, since)
	if err != nil {
		if errors.Is(err, ws.ErrRoomNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, struct {
		Messages []ws.ChatMessage `json:"messages"`
		NextSeq  int64            `json:"nextSeq"`
	}{Messages: history, NextSeq: nextSeq}, http.StatusOK)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}

func (s *Server) writeJSON(w http.ResponseWriter, payload any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// Start launches the HTTP server on the provided address.
func (s *Server) Start(addr string) *http.Server {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	return srv
}
