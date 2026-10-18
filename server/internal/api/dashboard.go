package api

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/thalisonnunes20/meshly/server/internal/ws"
)

//go:embed dashboard.html
var dashboardHTML string

func (s *Server) isAdminAuthenticated(r *http.Request) bool {
	// Check cookie
	if cookie, err := r.Cookie("meshly_session"); err == nil {
		if cookie.Value == "authenticated" {
			return true
		}
	}

	// Check Authorization header (Bearer <password>)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == s.cfg.AdminPassword {
			return true
		}
	}

	return false
}

type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req AdminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username != s.cfg.AdminUser || req.Password != s.cfg.AdminPassword {
		writeError(w, http.StatusUnauthorized, "usuário ou senha incorretos")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "meshly_session",
		Value:    "authenticated",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "user": req.Username})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "meshly_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "não autenticado")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"user":          s.cfg.AdminUser,
	})
}

func (s *Server) handleAdminListRooms(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "não autenticado")
		return
	}

	rooms := s.store.ListAllRooms()

	type PeerInfo struct {
		PublicKey string `json:"public_key"`
		Nickname  string `json:"nickname"`
		VirtualIP string `json:"virtual_ip"`
		IsOnline  bool   `json:"is_online"`
		JoinedAt  string `json:"joined_at"`
	}

	type RoomInfo struct {
		ID         string     `json:"id"`
		Name       string     `json:"name"`
		SubnetCIDR string     `json:"subnet_cidr"`
		CreatedAt  string     `json:"created_at"`
		Peers      []PeerInfo `json:"peers"`
	}

	result := make([]RoomInfo, 0, len(rooms))
	for _, r := range rooms {
		peersList := r.ListPeers()
		peers := make([]PeerInfo, 0, len(peersList))
		for _, p := range peersList {
			peers = append(peers, PeerInfo{
				PublicKey: p.PublicKey,
				Nickname:  p.Nickname,
				VirtualIP: p.VirtualIP,
				IsOnline:  p.IsOnline,
				JoinedAt:  p.JoinedAt.Format("02/01/2006 15:04"),
			})
		}

		result = append(result, RoomInfo{
			ID:         r.ID,
			Name:       r.Name,
			SubnetCIDR: r.SubnetCIDR,
			CreatedAt:  r.CreatedAt.Format("02/01/2006 15:04"),
			Peers:      peers,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminDeleteRoom(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "não autenticado")
		return
	}

	roomID := r.PathValue("id")
	room, err := s.store.DeleteRoom(roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Remove all peers in room from WireGuard
	for _, p := range room.Peers {
		_ = s.wg.RemovePeer(p.PublicKey)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAdminKickPeer(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "não autenticado")
		return
	}

	roomID := r.PathValue("id")
	pubKeyEscaped := r.PathValue("pubkey")

	pubKey, err := url.PathUnescape(pubKeyEscaped)
	if err != nil {
		pubKey = pubKeyEscaped
	}

	peer, err := s.store.LeaveRoom(roomID, pubKey)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Remove from WireGuard kernel interface
	_ = s.wg.RemovePeer(pubKey)

	// Broadcast peer left via websocket
	s.hub.BroadcastToRoom(roomID, &ws.Event{
		Type:    "peer_left",
		RoomID:  roomID,
		Payload: peer,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "kicked", "public_key": pubKey})
}

type UpdateRoomRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (s *Server) handleAdminUpdateRoom(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "não autenticado")
		return
	}

	roomID := r.PathValue("id")
	var req UpdateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	room, err := s.store.UpdateRoom(roomID, req.Name, req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
		"id":     room.ID,
		"name":   room.Name,
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
}
