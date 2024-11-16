// File: cmd/processor/main.go

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

	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"

	"toncenter-block-cacher/internal/api"
	"toncenter-block-cacher/internal/config"
	"toncenter-block-cacher/internal/monitor"
)

func main() {
	// Initialize logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting TON Block Cacher...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize TON client
	tonClient, err := initializeTONClient()
	if err != nil {
		log.Fatalf("Failed to initialize TON client: %v", err)
	}

	// Create data directory
	if err := os.MkdirAll(cfg.BlocksSavePath, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize monitor
	blockMonitor, err := monitor.NewMonitor(cfg, tonClient)
	if err != nil {
		log.Fatalf("Failed to create block monitor: %v", err)
	}

	// Initialize HTTP server
	httpServer := api.NewServer(blockMonitor.GetFileSystem(), cfg)
	server := &http.Server{
		Handler:      httpServer.GetRouter(),
		Addr:         fmt.Sprintf("%s:%s", cfg.HTTPHost, cfg.HTTPPort),
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
	}

	// Start HTTP server
	go func() {
		log.Printf("Starting HTTP server on %s:%s", cfg.HTTPHost, cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start block monitor
	if err := blockMonitor.Start(); err != nil {
		log.Fatalf("Failed to start block monitor: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	log.Printf("Received shutdown signal: %v", sig)

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop HTTP server
	log.Println("Shutting down HTTP server...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Stop block monitor
	log.Println("Stopping block monitor...")
	blockMonitor.Stop()

	log.Println("Shutdown complete")
}

func initializeTONClient() (*ton.APIClient, error) {
	conn := liteclient.NewConnectionPool()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.AddConnectionsFromConfigUrl(ctx, "https://ton-blockchain.github.io/global.config.json"); err != nil {
		return nil, fmt.Errorf("failed to add connections: %w", err)
	}

	// Create API client without type conversion
	client := ton.NewAPIClient(conn)

	return client, nil
}
