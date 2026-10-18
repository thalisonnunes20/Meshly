package config

import (
	"os"
	"strconv"
)

type Config struct {
	APIPort        int
	WGPort         int
	WGInterface    string
	PublicEndpoint string // Host:Port to advertise to clients
	VirtualCIDR    string
	DataDir        string
	AdminUser      string
	AdminPassword  string
}

func Load() *Config {
	apiPort := getEnvInt("MESHLY_API_PORT", 8080)
	wgPort := getEnvInt("MESHLY_WG_PORT", 51820)
	wgInterface := getEnvString("MESHLY_WG_INTERFACE", "wg0")
	publicEndpoint := getEnvString("MESHLY_PUBLIC_ENDPOINT", "")
	virtualCIDR := getEnvString("MESHLY_VIRTUAL_CIDR", "26.0.0.0/8")
	dataDir := getEnvString("MESHLY_DATA_DIR", "/etc/meshly")
	adminUser := getEnvString("MESHLY_ADMIN_USER", "admin")
	adminPassword := getEnvString("MESHLY_ADMIN_PASSWORD", "admin123")

	return &Config{
		APIPort:        apiPort,
		WGPort:         wgPort,
		WGInterface:    wgInterface,
		PublicEndpoint: publicEndpoint,
		VirtualCIDR:    virtualCIDR,
		DataDir:        dataDir,
		AdminUser:      adminUser,
		AdminPassword:  adminPassword,
	}
}

func getEnvString(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}
