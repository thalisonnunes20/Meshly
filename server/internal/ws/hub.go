package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thalisonnunes20/meshly/server/internal/store"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for desktop client
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Event struct {
	Type    string      `json:"type"` // "peer_joined", "peer_left", "peer_status", "heartbeat"
	RoomID  string      `json:"room_id"`
	Payload interface{} `json:"payload"`
}

type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	publicKey string
	roomID    string
}

type Hub struct {
	store      *store.Store
	clients    map[*Client]bool
	rooms      map[string]map[*Client]bool // roomID -> clients
	register   chan *Client
	unregister chan *Client
	broadcast  chan *Event
	mu         sync.RWMutex
}

func NewHub(st *store.Store) *Hub {
	return &Hub{
		store:      st,
		clients:    make(map[*Client]bool),
		rooms:      make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Event, 256),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			if _, ok := h.rooms[client.roomID]; !ok {
				h.rooms[client.roomID] = make(map[*Client]bool)
			}
			h.rooms[client.roomID][client] = true
			h.mu.Unlock()

			// Update peer online status
			peer := h.store.SetPeerStatus(client.publicKey, true)
			if peer != nil {
				h.BroadcastToRoom(client.roomID, &Event{
					Type:    "peer_status",
					RoomID:  client.roomID,
					Payload: peer,
				})
			}

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				if roomClients, ok := h.rooms[client.roomID]; ok {
					delete(roomClients, client)
					if len(roomClients) == 0 {
						delete(h.rooms, client.roomID)
					}
				}
			}
			h.mu.Unlock()

			// Update peer offline status
			peer := h.store.SetPeerStatus(client.publicKey, false)
			if peer != nil {
				h.BroadcastToRoom(client.roomID, &Event{
					Type:    "peer_status",
					RoomID:  client.roomID,
					Payload: peer,
				})
			}

		case event := <-h.broadcast:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			h.mu.RLock()
			roomClients := h.rooms[event.RoomID]
			for client := range roomClients {
				select {
				case client.send <- data:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) BroadcastToRoom(roomID string, event *Event) {
	h.broadcast <- event
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	pubKey := r.URL.Query().Get("pubkey")
	roomID := r.URL.Query().Get("room")

	if pubKey == "" || roomID == "" {
		http.Error(w, "missing pubkey or room parameter", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:       h,
		conn:      conn,
		send:      make(chan []byte, 64),
		publicKey: pubKey,
		roomID:    roomID,
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// Handle heartbeat or incoming ping
		var evt Event
		if err := json.Unmarshal(message, &evt); err == nil && evt.Type == "heartbeat" {
			c.hub.store.SetPeerStatus(c.publicKey, true)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
