package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/crypto/curve25519"
)

type Manager struct {
	iface          string
	port           int
	dataDir        string
	privateKey     string
	publicKey      string
	isLinux        bool
	mu             sync.Mutex
	activeSubnets  map[string]bool // keep track of added iptables rules
}

// GenerateKeyPair generates a standard WireGuard Curve25519 private and public key pair in pure Go
func GenerateKeyPair() (privateKeyBase64, publicKeyBase64 string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Clamp the private key according to WireGuard / Curve25519 spec
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:]), nil
}

func NewManager(iface string, port int, dataDir string) (*Manager, error) {
	m := &Manager{
		iface:         iface,
		port:          port,
		dataDir:       dataDir,
		isLinux:       runtime.GOOS == "linux",
		activeSubnets: make(map[string]bool),
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	// Load or generate server keys
	privKey, pubKey, err := m.loadOrGenerateKeys()
	if err != nil {
		return nil, err
	}
	m.privateKey = privKey
	m.publicKey = pubKey

	return m, nil
}

func (m *Manager) PublicKey() string {
	return m.publicKey
}

func (m *Manager) loadOrGenerateKeys() (string, string, error) {
	privPath := filepath.Join(m.dataDir, "server_private.key")
	pubPath := filepath.Join(m.dataDir, "server_public.key")

	privBytes, err1 := os.ReadFile(privPath)
	pubBytes, err2 := os.ReadFile(pubPath)

	if err1 == nil && err2 == nil && len(privBytes) > 0 && len(pubBytes) > 0 {
		return string(privBytes), string(pubBytes), nil
	}

	// Generate new keys
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		return "", "", err
	}

	if err := os.WriteFile(privPath, []byte(priv), 0600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(pubPath, []byte(pub), 0644); err != nil {
		return "", "", err
	}

	log.Printf("[WireGuard] Generated new server key pair. Public key: %s", pub)
	return priv, pub, nil
}

// Start configures the WireGuard interface and firewall rules on Linux
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isLinux {
		log.Printf("[WireGuard] Running in development mode (non-Linux OS: %s). WireGuard kernel operations mocked.", runtime.GOOS)
		return nil
	}

	log.Printf("[WireGuard] Initializing WireGuard interface %s on port %d...", m.iface, m.port)

	// 1. Remove existing interface if present
	_ = exec.Command("ip", "link", "del", "dev", m.iface).Run()

	// 2. Add wireguard interface
	if out, err := exec.Command("ip", "link", "add", "dev", m.iface, "type", "wireguard").CombinedOutput(); err != nil {
		log.Printf("[WireGuard] Warning creating wireguard interface: %s (%v)", string(out), err)
	}

	// 3. Set address (26.0.0.1/8 for entire mesh space)
	if out, err := exec.Command("ip", "address", "add", "26.0.0.1/8", "dev", m.iface).CombinedOutput(); err != nil {
		log.Printf("[WireGuard] Warning assigning IP: %s (%v)", string(out), err)
	}

	// 4. Save private key to temp file for wg set
	keyFile := filepath.Join(m.dataDir, "tmp_priv.key")
	_ = os.WriteFile(keyFile, []byte(m.privateKey), 0600)
	defer os.Remove(keyFile)

	// 5. Configure wg interface
	if out, err := exec.Command("wg", "set", m.iface, "listen-port", fmt.Sprintf("%d", m.port), "private-key", keyFile).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to configure wg interface: %s (%w)", string(out), err)
	}

	// 6. Up interface
	if out, err := exec.Command("ip", "link", "set", "up", "dev", m.iface).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up wg interface: %s (%w)", string(out), err)
	}

	// 7. Setup base iptables isolation
	m.setupBaseFirewall()

	log.Printf("[WireGuard] Interface %s successfully started and listening on UDP port %d", m.iface, m.port)
	return nil
}

func (m *Manager) setupBaseFirewall() {
	// Clean previous rules if any
	_ = exec.Command("iptables", "-D", "FORWARD", "-i", m.iface, "-o", m.iface, "-j", "DROP").Run()
	// Default drop for any forwarding on wg0 unless explicitly accepted for same room subnet
	_ = exec.Command("iptables", "-A", "FORWARD", "-i", m.iface, "-o", m.iface, "-j", "DROP").Run()
}

// AddPeer registers a client's public key with its allocated virtual IP
func (m *Manager) AddPeer(pubKey, virtualIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isLinux {
		log.Printf("[WireGuard Mock] AddPeer: %s -> %s/32", pubKey, virtualIP)
		return nil
	}

	// wg set wg0 peer <pubKey> allowed-ips <virtualIP>/32
	allowedIPs := fmt.Sprintf("%s/32", virtualIP)
	cmd := exec.Command("wg", "set", m.iface, "peer", pubKey, "allowed-ips", allowedIPs)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg set peer failed: %s (%w)", string(out), err)
	}

	log.Printf("[WireGuard] Peer added: %s (%s)", pubKey, allowedIPs)
	return nil
}

// RemovePeer removes a client from WireGuard interface
func (m *Manager) RemovePeer(pubKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isLinux {
		log.Printf("[WireGuard Mock] RemovePeer: %s", pubKey)
		return nil
	}

	cmd := exec.Command("wg", "set", m.iface, "peer", pubKey, "remove")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg remove peer failed: %s (%w)", string(out), err)
	}

	log.Printf("[WireGuard] Peer removed: %s", pubKey)
	return nil
}

// AllowRoomSubnet configures iptables so peers within subnetCIDR can communicate with each other
func (m *Manager) AllowRoomSubnet(subnetCIDR string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeSubnets[subnetCIDR] {
		return nil
	}

	if !m.isLinux {
		log.Printf("[WireGuard Mock] AllowRoomSubnet: %s", subnetCIDR)
		m.activeSubnets[subnetCIDR] = true
		return nil
	}

	// Insert accept rule at position 1 (before default DROP)
	// iptables -I FORWARD 1 -i wg0 -o wg0 -s <subnetCIDR> -d <subnetCIDR> -j ACCEPT
	cmd := exec.Command("iptables", "-I", "FORWARD", "1", "-i", m.iface, "-o", m.iface, "-s", subnetCIDR, "-d", subnetCIDR, "-j", "ACCEPT")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables insert rule failed: %s (%w)", string(out), err)
	}

	m.activeSubnets[subnetCIDR] = true
	log.Printf("[WireGuard Firewall] Allowed isolated forwarding for subnet: %s", subnetCIDR)
	return nil
}
