package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/thalisonnunes20/meshly/server/internal/config"
	"github.com/thalisonnunes20/meshly/server/internal/store"
	"github.com/thalisonnunes20/meshly/server/internal/wireguard"
	"github.com/thalisonnunes20/meshly/server/internal/ws"
)

type Server struct {
	cfg   *config.Config
	store *store.Store
	wg    *wireguard.Manager
	hub   *ws.Hub
}

func NewServer(cfg *config.Config, st *store.Store, wg *wireguard.Manager, hub *ws.Hub) *Server {
	return &Server{
		cfg:   cfg,
		store: st,
		wg:    wg,
		hub:   hub,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	// Room operations
	mux.HandleFunc("POST /api/v1/rooms/create", s.handleCreateRoom)
	mux.HandleFunc("POST /api/v1/rooms/join", s.handleJoinRoom)
	mux.HandleFunc("POST /api/v1/rooms/leave", s.handleLeaveRoom)
	mux.HandleFunc("GET /api/v1/rooms/{id}/peers", s.handleListPeers)

	// WebSocket
	mux.HandleFunc("/api/v1/ws", s.hub.HandleWS)

	// Enable CORS for web development and cross-origin desktop clients
	return corsMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	endpoint := s.cfg.PublicEndpoint
	if endpoint == "" {
		// Use request host if public endpoint is not set
		host := r.Host
		if colon := strings.Index(host, ":"); colon != -1 {
			host = host[:colon]
		}
		endpoint = host
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":            "ok",
		"service":           "Meshly Control Plane",
		"version":           "0.1.0",
		"server_public_key": s.wg.PublicKey(),
		"wg_port":           s.cfg.WGPort,
		"endpoint":          endpoint,
	})
}

type CreateRoomRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Name) == "" || len(req.Password) < 3 {
		writeError(w, http.StatusBadRequest, "name and password (min 3 chars) are required")
		return
	}

	room, err := s.store.CreateRoom(req.Name, req.Password)
	if err != nil {
		if err == store.ErrRoomAlreadyExists {
			writeError(w, http.StatusConflict, "a room with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Prepare firewall rule for room subnet
	_ = s.wg.AllowRoomSubnet(room.SubnetCIDR)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"room_id":     room.ID,
		"name":        room.Name,
		"subnet_cidr": room.SubnetCIDR,
	})
}

type JoinRoomRequest struct {
	RoomName  string `json:"room_name"`
	Password  string `json:"password"`
	PublicKey string `json:"public_key"`
	Nickname  string `json:"nickname"`
}

type JoinRoomResponse struct {
	Status          string        `json:"status"`
	RoomID          string        `json:"room_id"`
	RoomName        string        `json:"room_name"`
	SubnetCIDR      string        `json:"subnet_cidr"`
	VirtualIP       string        `json:"virtual_ip"`       // Client's virtual IP (e.g. 26.0.1.2)
	VirtualCIDR     string        `json:"virtual_cidr"`     // Client's IP with mask (e.g. 26.0.1.2/24)
	ServerEndpoint  string        `json:"server_endpoint"`  // Host:51820
	ServerPublicKey string        `json:"server_public_key"`
	Peers           []*store.Peer `json:"peers"`
}

func (s *Server) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var req JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.RoomName) == "" || req.PublicKey == "" || req.Nickname == "" {
		writeError(w, http.StatusBadRequest, "room_name, public_key and nickname are required")
		return
	}

	room, peer, err := s.store.JoinRoom(req.RoomName, req.Password, req.PublicKey, req.Nickname)
	if err != nil {
		switch err {
		case store.ErrRoomNotFound:
			writeError(w, http.StatusNotFound, "room not found")
		case store.ErrInvalidPassword:
			writeError(w, http.StatusUnauthorized, "invalid password")
		case store.ErrRoomFull:
			writeError(w, http.StatusInsufficientStorage, "room is full")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Add peer to WireGuard kernel interface
	if err := s.wg.AddPeer(peer.PublicKey, peer.VirtualIP); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to configure wireguard peer: "+err.Error())
		return
	}

	// Ensure firewall isolation rule exists for this room's subnet
	_ = s.wg.AllowRoomSubnet(room.SubnetCIDR)

	// Broadcast peer joined to everyone in the room via WebSocket
	s.hub.BroadcastToRoom(room.ID, &ws.Event{
		Type:    "peer_joined",
		RoomID:  room.ID,
		Payload: peer,
	})

	// Determine endpoint to send to client
	host := s.cfg.PublicEndpoint
	if host == "" {
		reqHost := r.Host
		if colon := strings.Index(reqHost, ":"); colon != -1 {
			reqHost = reqHost[:colon]
		}
		host = reqHost
	}

	serverEndpoint := host
	if !strings.Contains(serverEndpoint, ":") {
		serverEndpoint = serverEndpoint + ":51820"
	}

	writeJSON(w, http.StatusOK, JoinRoomResponse{
		Status:          "success",
		RoomID:          room.ID,
		RoomName:        room.Name,
		SubnetCIDR:      room.SubnetCIDR,
		VirtualIP:       peer.VirtualIP,
		VirtualCIDR:     peer.VirtualIP + "/24",
		ServerEndpoint:  serverEndpoint,
		ServerPublicKey: s.wg.PublicKey(),
		Peers:           room.ListPeers(),
	})
}

type LeaveRoomRequest struct {
	RoomID    string `json:"room_id"`
	PublicKey string `json:"public_key"`
}

func (s *Server) handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	var req LeaveRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	peer, err := s.store.LeaveRoom(req.RoomID, req.PublicKey)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Remove from WireGuard interface
	_ = s.wg.RemovePeer(req.PublicKey)

	// Broadcast peer left
	s.hub.BroadcastToRoom(req.RoomID, &ws.Event{
		Type:    "peer_left",
		RoomID:  req.RoomID,
		Payload: peer,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

func (s *Server) handleListPeers(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	room, err := s.store.GetRoomByID(roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	writeJSON(w, http.StatusOK, room.ListPeers())
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
