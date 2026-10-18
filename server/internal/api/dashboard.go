package api

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/thalisonnunes20/meshly/server/internal/ws"
)

//go:embed dashboard.html
var dashboardHTML string

func (s *Server) handleAdminListRooms(w http.ResponseWriter, r *http.Request) {
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
