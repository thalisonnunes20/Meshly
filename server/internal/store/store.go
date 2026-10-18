package store

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrRoomNotFound      = errors.New("room not found")
	ErrRoomAlreadyExists = errors.New("room already exists")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrRoomFull          = errors.New("room subnet is full (max 250 peers per room)")
	ErrPeerNotFound      = errors.New("peer not found")
)

type Peer struct {
	PublicKey string    `json:"public_key"`
	Nickname  string    `json:"nickname"`
	VirtualIP string    `json:"virtual_ip"` // e.g. "26.0.1.2"
	RoomID    string    `json:"room_id"`
	JoinedAt  time.Time `json:"joined_at"`
	LastSeen  time.Time `json:"last_seen"`
	IsOnline  bool      `json:"is_online"`
}

type Room struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	PasswordHash string           `json:"-"`
	SubnetIndex  int              `json:"subnet_index"` // e.g. 1 => 26.0.1.0/24
	SubnetCIDR   string           `json:"subnet_cidr"`  // "26.0.1.0/24"
	CreatedAt    time.Time        `json:"created_at"`
	Peers        map[string]*Peer `json:"peers"` // key: PublicKey
	mu           sync.RWMutex
}

type Store struct {
	mu           sync.RWMutex
	rooms        map[string]*Room // key: room name lowercase
	roomsByID    map[string]*Room // key: room id
	peersByPub   map[string]*Peer // key: public key
	nextSubnetID int
}

func New() *Store {
	return &Store{
		rooms:        make(map[string]*Room),
		roomsByID:    make(map[string]*Room),
		peersByPub:   make(map[string]*Peer),
		nextSubnetID: 1, // Start subnet at 1 (26.0.1.0/24)
	}
}

// CreateRoom registers a new virtual network room
func (s *Store) CreateRoom(name, password string) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normName := normalize(name)
	if _, exists := s.rooms[normName]; exists {
		return nil, ErrRoomAlreadyExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	subnetIdx := s.nextSubnetID
	s.nextSubnetID++

	// Format: 26.0.{subnetIdx}.0/24
	subnetCIDR := fmt.Sprintf("26.0.%d.0/24", subnetIdx)
	roomID := fmt.Sprintf("room-%d", subnetIdx)

	room := &Room{
		ID:           roomID,
		Name:         name,
		PasswordHash: string(hash),
		SubnetIndex:  subnetIdx,
		SubnetCIDR:   subnetCIDR,
		CreatedAt:    time.Now().UTC(),
		Peers:        make(map[string]*Peer),
	}

	s.rooms[normName] = room
	s.roomsByID[roomID] = room
	return room, nil
}

// GetRoom retrieves a room by name
func (s *Store) GetRoom(name string) (*Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.rooms[normalize(name)]
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room, nil
}

// GetRoomByID retrieves a room by ID
func (s *Store) GetRoomByID(id string) (*Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.roomsByID[id]
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room, nil
}

// JoinRoom authenticates a user into a room and allocates a virtual IP
func (s *Store) JoinRoom(roomName, password, publicKey, nickname string) (*Room, *Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[normalize(roomName)]
	if !exists {
		return nil, nil, ErrRoomNotFound
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(room.PasswordHash), []byte(password)); err != nil {
		return nil, nil, ErrInvalidPassword
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	// Check if peer is already in this room
	if existingPeer, ok := room.Peers[publicKey]; ok {
		existingPeer.Nickname = nickname
		existingPeer.LastSeen = time.Now().UTC()
		existingPeer.IsOnline = true
		s.peersByPub[publicKey] = existingPeer
		return room, existingPeer, nil
	}

	// Allocate next IP inside 26.0.{subnetIdx}.X (host range 2..254)
	usedHostOctets := make(map[int]bool)
	for _, p := range room.Peers {
		var o1, o2, o3, o4 int
		fmt.Sscanf(p.VirtualIP, "%d.%d.%d.%d", &o1, &o2, &o3, &o4)
		usedHostOctets[o4] = true
	}

	allocatedHost := -1
	for h := 2; h <= 254; h++ { // .1 is reserved for virtual gateway/server, peers start from .2
		if !usedHostOctets[h] {
			allocatedHost = h
			break
		}
	}

	if allocatedHost == -1 {
		return nil, nil, ErrRoomFull
	}

	virtualIP := fmt.Sprintf("26.0.%d.%d", room.SubnetIndex, allocatedHost)

	peer := &Peer{
		PublicKey: publicKey,
		Nickname:  nickname,
		VirtualIP: virtualIP,
		RoomID:    room.ID,
		JoinedAt:  time.Now().UTC(),
		LastSeen:  time.Now().UTC(),
		IsOnline:  true,
	}

	room.Peers[publicKey] = peer
	s.peersByPub[publicKey] = peer

	return room, peer, nil
}

// LeaveRoom removes a peer from a room
func (s *Store) LeaveRoom(roomID, publicKey string) (*Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.roomsByID[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	peer, exists := room.Peers[publicKey]
	if !exists {
		return nil, ErrPeerNotFound
	}

	delete(room.Peers, publicKey)
	delete(s.peersByPub, publicKey)
	return peer, nil
}

// SetPeerStatus updates peer online status
func (s *Store) SetPeerStatus(publicKey string, online bool) *Peer {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, exists := s.peersByPub[publicKey]
	if exists {
		peer.IsOnline = online
		peer.LastSeen = time.Now().UTC()
	}
	return peer
}

// ListPeers returns a slice of peers in the room
func (r *Room) ListPeers() []*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Peer, 0, len(r.Peers))
	for _, p := range r.Peers {
		list = append(list, p)
	}
	return list
}

// ListAllRooms returns all registered rooms with their peers
func (s *Store) ListAllRooms() []*Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]*Room, 0, len(s.roomsByID))
	for _, room := range s.roomsByID {
		rooms = append(rooms, room)
	}
	return rooms
}

// DeleteRoom completely deletes a room and returns it
func (s *Store) DeleteRoom(roomID string) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.roomsByID[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}

	room.mu.Lock()
	for pubKey := range room.Peers {
		delete(s.peersByPub, pubKey)
	}
	room.mu.Unlock()

	delete(s.roomsByID, roomID)
	delete(s.rooms, normalize(room.Name))
	return room, nil
}

// UpdateRoom updates a room's name or password
func (s *Store) UpdateRoom(roomID, newName, newPassword string) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.roomsByID[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}

	if newName != "" && normalize(newName) != normalize(room.Name) {
		normNew := normalize(newName)
		if _, taken := s.rooms[normNew]; taken {
			return nil, ErrRoomAlreadyExists
		}
		delete(s.rooms, normalize(room.Name))
		room.Name = newName
		s.rooms[normNew] = room
	}

	if newPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		room.PasswordHash = string(hash)
	}

	return room, nil
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

