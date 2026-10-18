package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thalisonnunes20/meshly/server/internal/api"
	"github.com/thalisonnunes20/meshly/server/internal/config"
	"github.com/thalisonnunes20/meshly/server/internal/store"
	"github.com/thalisonnunes20/meshly/server/internal/wireguard"
	"github.com/thalisonnunes20/meshly/server/internal/ws"
)

func main() {
	log.Println("==================================================")
	log.Println("           MESHLY CONTROL PLANE SERVER           ")
	log.Println("   WireGuard-based Virtual LAN (Hamachi/Radmin)   ")
	log.Println("==================================================")

	cfg := config.Load()
	log.Printf("[Config] API Port: %d | WG Port: %d | Interface: %s", cfg.APIPort, cfg.WGPort, cfg.WGInterface)

	// Initialize Store
	st := store.New()

	// Initialize WireGuard Manager
	wgMgr, err := wireguard.NewManager(cfg.WGInterface, cfg.WGPort, cfg.DataDir)
	if err != nil {
		log.Fatalf("[WireGuard] Failed to initialize manager: %v", err)
	}

	if err := wgMgr.Start(); err != nil {
		log.Fatalf("[WireGuard] Failed to start interface: %v", err)
	}

	// Initialize WebSocket Hub
	hub := ws.NewHub(st)
	go hub.Run()

	// Initialize HTTP API
	apiServer := api.NewServer(cfg, st, wgMgr, hub)
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.APIPort)

	srv := &http.Server{
		Addr:         addr,
		Handler:      apiServer.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown channel
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[API] Server listening on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[API] Listen error: %v", err)
		}
	}()

	<-shutdown
	log.Println("\n[Shutdown] Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[Shutdown] Server shutdown error: %v", err)
	}

	log.Println("[Shutdown] Meshly server stopped.")
}
