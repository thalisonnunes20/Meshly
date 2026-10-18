package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/curve25519"
)

type StoredConfig struct {
	Endpoint string `json:"endpoint"`
	Nickname string `json:"nickname"`
}

type KeyPair struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

type ServerHealth struct {
	Status          string `json:"status"`
	Service         string `json:"service"`
	Version         string `json:"version"`
	ServerPublicKey string `json:"server_public_key"`
	WGPort          int    `json:"wg_port"`
	Endpoint        string `json:"endpoint"`
	ClientPublicKey string `json:"client_public_key"`
}

type Peer struct {
	PublicKey string `json:"public_key"`
	Nickname  string `json:"nickname"`
	VirtualIP string `json:"virtual_ip"`
	RoomID    string `json:"room_id"`
	IsOnline  bool   `json:"is_online"`
	PingMs    int    `json:"ping_ms"`
}

type RoomState struct {
	RoomID          string `json:"room_id"`
	RoomName        string `json:"room_name"`
	SubnetCIDR      string `json:"subnet_cidr"`
	VirtualIP       string `json:"virtual_ip"`
	VirtualCIDR     string `json:"virtual_cidr"`
	ServerEndpoint  string `json:"server_endpoint"`
	ServerPublicKey string `json:"server_public_key"`
	Peers           []Peer `json:"peers"`
}

type App struct {
	ctx            context.Context
	mu             sync.RWMutex
	endpoint       string
	nickname       string
	keys           KeyPair
	currentRoom    *RoomState
	wsConn         *websocket.Conn
	isTunnelActive bool
	appDir         string
}

func NewApp() *App {
	appDir := filepath.Join(os.Getenv("APPDATA"), "Meshly")
	if os.Getenv("APPDATA") == "" {
		home, _ := os.UserHomeDir()
		appDir = filepath.Join(home, ".meshly")
	}
	_ = os.MkdirAll(appDir, 0700)

	return &App{
		appDir: appDir,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.loadOrGenerateKeys()
}

func (a *App) loadOrGenerateKeys() {
	keyFile := filepath.Join(a.appDir, "keys.json")
	if data, err := os.ReadFile(keyFile); err == nil {
		var kp KeyPair
		if err := json.Unmarshal(data, &kp); err == nil && kp.PrivateKey != "" && kp.PublicKey != "" {
			a.keys = kp
			return
		}
	}

	// Generate Curve25519 keypair for WireGuard
	var priv [32]byte
	_, _ = rand.Read(priv[:])
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	kp := KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(priv[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(pub[:]),
	}
	a.keys = kp

	if data, err := json.MarshalIndent(kp, "", "  "); err == nil {
		_ = os.WriteFile(keyFile, data, 0600)
	}
}

// GetStoredConfig loads last used endpoint and nickname
func (a *App) GetStoredConfig() StoredConfig {
	cfgFile := filepath.Join(a.appDir, "config.json")
	var cfg StoredConfig
	if data, err := os.ReadFile(cfgFile); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "192.168.10.2:8080"
	}
	if cfg.Nickname == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "Meu-PC"
		}
		cfg.Nickname = hostname
	}
	return cfg
}

// SaveStoredConfig saves endpoint and nickname
func (a *App) SaveStoredConfig(endpoint, nickname string) {
	cfg := StoredConfig{
		Endpoint: strings.TrimSpace(endpoint),
		Nickname: strings.TrimSpace(nickname),
	}
	cfgFile := filepath.Join(a.appDir, "config.json")
	if data, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		_ = os.WriteFile(cfgFile, data, 0600)
	}
}

// ConnectServer verifies health of the Meshly server
func (a *App) ConnectServer(rawEndpoint, nickname string) (*ServerHealth, error) {
	rawEndpoint = strings.TrimSpace(rawEndpoint)
	nickname = strings.TrimSpace(nickname)

	if rawEndpoint == "" {
		return nil, errors.New("endereço do servidor não pode ser vazio")
	}
	if nickname == "" {
		return nil, errors.New("apelido não pode ser vazio")
	}

	// Format base URL
	baseURL := rawEndpoint
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/v1/health")
	if err != nil {
		return nil, fmt.Errorf("não foi possível conectar ao servidor (%s): %w", rawEndpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("servidor respondeu com status HTTP %d", resp.StatusCode)
	}

	var health ServerHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("resposta inválida do servidor: %w", err)
	}

	a.mu.Lock()
	a.endpoint = baseURL
	a.nickname = nickname
	a.mu.Unlock()

	a.SaveStoredConfig(rawEndpoint, nickname)

	health.ClientPublicKey = a.keys.PublicKey
	return &health, nil
}

// CreateRoom creates a new virtual network room
func (a *App) CreateRoom(name, password string) (map[string]interface{}, error) {
	a.mu.RLock()
	endpoint := a.endpoint
	a.mu.RUnlock()

	if endpoint == "" {
		return nil, errors.New("não conectado a um servidor")
	}

	payload := map[string]string{
		"name":     strings.TrimSpace(name),
		"password": password,
	}
	data, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(endpoint+"/api/v1/rooms/create", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar sala: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		if errMsg, ok := result["error"].(string); ok {
			return nil, errors.New(errMsg)
		}
		return nil, fmt.Errorf("falha ao criar sala (código %d)", resp.StatusCode)
	}

	return result, nil
}

// JoinRoom joins an existing room and gets assigned a virtual IP
func (a *App) JoinRoom(roomName, password string) (*RoomState, error) {
	a.mu.RLock()
	endpoint := a.endpoint
	nickname := a.nickname
	pubKey := a.keys.PublicKey
	a.mu.RUnlock()

	if endpoint == "" {
		return nil, errors.New("não conectado a um servidor")
	}

	payload := map[string]string{
		"room_name":  strings.TrimSpace(roomName),
		"password":   password,
		"public_key": pubKey,
		"nickname":   nickname,
	}
	data, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(endpoint+"/api/v1/rooms/join", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar na sala: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status          string `json:"status"`
		RoomID          string `json:"room_id"`
		RoomName        string `json:"room_name"`
		SubnetCIDR      string `json:"subnet_cidr"`
		VirtualIP       string `json:"virtual_ip"`
		VirtualCIDR     string `json:"virtual_cidr"`
		ServerEndpoint  string `json:"server_endpoint"`
		ServerPublicKey string `json:"server_public_key"`
		Peers           []Peer `json:"peers"`
		Error           string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		if result.Error != "" {
			return nil, errors.New(result.Error)
		}
		return nil, fmt.Errorf("falha ao entrar na sala (código %d)", resp.StatusCode)
	}

	roomState := &RoomState{
		RoomID:          result.RoomID,
		RoomName:        result.RoomName,
		SubnetCIDR:      result.SubnetCIDR,
		VirtualIP:       result.VirtualIP,
		VirtualCIDR:     result.VirtualCIDR,
		ServerEndpoint:  result.ServerEndpoint,
		ServerPublicKey: result.ServerPublicKey,
		Peers:           result.Peers,
	}

	a.mu.Lock()
	a.currentRoom = roomState
	a.isTunnelActive = true
	a.mu.Unlock()

	// Generate client wireguard config file locally
	a.generateWireGuardConfig(roomState)

	// Connect WebSocket for real-time room events
	go a.startWebSocket(endpoint, roomState.RoomID, pubKey)

	return roomState, nil
}

func (a *App) generateWireGuardConfig(r *RoomState) {
	conf := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = 25
`, a.keys.PrivateKey, r.VirtualCIDR, r.ServerPublicKey, r.ServerEndpoint, r.SubnetCIDR)

	confPath := filepath.Join(a.appDir, "meshly.conf")
	_ = os.WriteFile(confPath, []byte(conf), 0600)
}

func (a *App) startWebSocket(endpoint, roomID, pubKey string) {
	a.mu.Lock()
	if a.wsConn != nil {
		_ = a.wsConn.Close()
		a.wsConn = nil
	}
	a.mu.Unlock()

	u, err := url.Parse(endpoint)
	if err != nil {
		return
	}

	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/api/v1/ws?pubkey=%s&room=%s", wsScheme, u.Host, url.QueryEscape(pubKey), url.QueryEscape(roomID))

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return
	}

	a.mu.Lock()
	a.wsConn = conn
	a.mu.Unlock()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var event struct {
			Type    string          `json:"type"`
			RoomID  string          `json:"room_id"`
			Payload json.RawMessage `json:"payload"`
		}

		if err := json.Unmarshal(message, &event); err == nil {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "room_event", event)
			}
		}
	}
}

// LeaveRoom exits current room
func (a *App) LeaveRoom() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.currentRoom == nil {
		return nil
	}

	if a.wsConn != nil {
		_ = a.wsConn.Close()
		a.wsConn = nil
	}

	payload := map[string]string{
		"room_id":    a.currentRoom.RoomID,
		"public_key": a.keys.PublicKey,
	}
	data, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 3 * time.Second}
	_, _ = client.Post(a.endpoint+"/api/v1/rooms/leave", "application/json", bytes.NewBuffer(data))

	a.currentRoom = nil
	a.isTunnelActive = false
	return nil
}

// ToggleTunnel toggles VPN tunnel state
func (a *App) ToggleTunnel(enable bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.isTunnelActive = enable
	return a.isTunnelActive
}

// PingPeer pings an IP and returns latency in milliseconds (-1 if unreachable)
func (a *App) PingPeer(ip string) int {
	cmd := exec.Command("ping", "-n", "1", "-w", "1000", ip)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return -1
	}

	re := regexp.MustCompile(`(?:tempo|time)[=<](\d+)ms`)
	match := re.FindStringSubmatch(string(out))
	if len(match) > 1 {
		if ms, err := strconv.Atoi(match[1]); err == nil {
			return ms
		}
	}
	return 1
}

// GetActiveRoom returns current room state
func (a *App) GetActiveRoom() *RoomState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentRoom
}
