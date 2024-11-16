package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"toncenter-block-cacher/internal/api"
	"toncenter-block-cacher/internal/config"
	"toncenter-block-cacher/internal/utils"
)

type BlockMonitor struct {
	tonAPI    *ton.APIClientWrapped
	tonCenter *api.Client
	fs        *utils.FileSystem
	cfg       *config.Config
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

func NewBlockMonitor(cfg *config.Config) (*BlockMonitor, error) {
	// Initialize TON connection
	conn := liteclient.NewConnectionPool()
	err := conn.AddConnectionsFromConfigUrl(context.Background(), "https://ton-blockchain.github.io/global.config.json")
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}

	// Initialize API clients
	tonAPI := ton.NewAPIClient(conn).WithRetry()
	tonCenter := api.NewClient(
		cfg.ApiTonCenterBaseURL,
		[]string{cfg.ApiTonCenterKEY},
		cfg.ApiTonCenterRPS,
	)

	// Initialize filesystem
	fs := utils.NewFileSystem(cfg.BlocksSavePath)

	return &BlockMonitor{
		tonAPI:    &tonAPI,
		tonCenter: tonCenter,
		fs:        fs,
		cfg:       cfg,
		stopChan:  make(chan struct{}),
	}, nil
}

func (m *BlockMonitor) Start() error {
	lastSavedBlock, err := m.fs.GetLastSavedBlockNumber()
	if err != nil {
		log.Printf("No saved blocks found, starting from current block")
		lastSavedBlock = 0
	}

	master, err := m.getMasterchainInfo()
	if err != nil {
		return fmt.Errorf("error getting initial masterchain info: %w", err)
	}

	var startBlock uint32
	if lastSavedBlock == 0 || master.SeqNo > lastSavedBlock+uint32(m.cfg.MaxSavedBlocks) {
		startBlock = master.SeqNo - uint32(m.cfg.MaxSavedBlocks)
		log.Printf("Starting from block %d (current: %d, max blocks: %d)",
			startBlock, master.SeqNo, m.cfg.MaxSavedBlocks)
	} else {
		startBlock = lastSavedBlock + 1
		log.Printf("Continuing from last saved block: %d", startBlock)
	}

	m.wg.Add(1)
	go m.monitorBlocks(startBlock)

	return nil
}

func (m *BlockMonitor) monitorBlocks(startBlock uint32) {
	defer m.wg.Done()

	lastProcessed := startBlock - 1
	processingQueue := make(chan uint32, m.cfg.MaxParallelFetches)
	var processWg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < m.cfg.MaxParallelFetches; i++ {
		processWg.Add(1)
		go m.blockProcessor(processingQueue, &processWg)
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			close(processingQueue)
			processWg.Wait()
			return

		case <-ticker.C:
			master, err := m.getMasterchainInfo()
			if err != nil {
				log.Printf("Error getting masterchain info: %v", err)
				continue
			}

			// Calculate safe block number (2 blocks behind the latest)
			safeBlock := master.SeqNo
			if safeBlock > 2 {
				safeBlock -= 2 // Stay 2 blocks behind to ensure availability
			}

			// Process new blocks up to the safe block
			for block := lastProcessed + 1; block <= safeBlock; block++ {
				select {
				case processingQueue <- block:
					lastProcessed = block
				case <-m.stopChan:
					close(processingQueue)
					processWg.Wait()
					return
				}
			}

			// Cleanup old blocks
			if err := m.removeOldBlocks(); err != nil {
				log.Printf("Error removing old blocks: %v", err)
			}
		}
	}
}

func (m *BlockMonitor) blockProcessor(queue chan uint32, wg *sync.WaitGroup) {
	defer wg.Done()

	for blockNum := range queue {
		retries := 0
		for {
			err := m.processBlock(blockNum)
			if err == nil {
				log.Printf("Successfully processed block %d", blockNum)
				break
			}

			if retries >= 3 {
				log.Printf("Failed to process block %d after %d retries: %v",
					blockNum, retries, err)
				break
			}

			retries++
			log.Printf("Retry %d for block %d: %v", retries, blockNum, err)
			time.Sleep(m.cfg.DelayOnRetry)
		}
	}
}

func (m *BlockMonitor) processBlock(block uint32) error {
	for retries := 0; retries < 3; retries++ {
		if retries > 0 {
			delay := time.Duration(1<<uint(retries)) * time.Second
			time.Sleep(delay)
		}

		response, err := m.tonCenter.FetchChunk(int(block), m.cfg.ApiFetchLimit, 0)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				// Block not available yet
				time.Sleep(2 * time.Second)
				continue
			}
			return fmt.Errorf("fetch from toncenter failed: %w", err)
		}

		// Validate response
		if response == nil {
			return fmt.Errorf("empty response from toncenter")
		}

		// Set the correct block numbers in events
		for i := range response.Chunk.Events {
			response.Chunk.Events[i].BlockNumber = block
		}

		// Marshal the processed data
		events, err := json.Marshal(response.Chunk.Events)
		if err != nil {
			return fmt.Errorf("marshal events failed: %w", err)
		}

		addresses, err := json.Marshal(response.Chunk.AddressBook)
		if err != nil {
			return fmt.Errorf("marshal addresses failed: %w", err)
		}

		// Save to filesystem
		err = m.fs.SaveBlockData(int(block), events, addresses)
		if err != nil {
			return fmt.Errorf("save block failed: %w", err)
		}

		return nil
	}

	return fmt.Errorf("block %d not available after retries", block)
}

func (m *BlockMonitor) getMasterchainInfo() (*ton.BlockIDExt, error) {
	master, err := (*m.tonAPI).GetMasterchainInfo(context.Background())
	if err != nil {
		return nil, fmt.Errorf("get masterchain info: %w", err)
	}
	return master, nil
}

func (m *BlockMonitor) removeOldBlocks() error {
	lastBlock, err := m.fs.GetLastSavedBlockNumber()
	if err != nil {
		return fmt.Errorf("get last block failed: %w", err)
	}

	if int(lastBlock) <= m.cfg.MaxSavedBlocks {
		return nil
	}

	startBlock := int(lastBlock) - m.cfg.MaxSavedBlocks
	return m.fs.RemoveBlocksBefore(startBlock)
}

func (m *BlockMonitor) Stop() {
	close(m.stopChan)
	m.wg.Wait()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting TON Block Cacher...")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Println("Configuration loaded successfully")

	if err := os.MkdirAll(cfg.BlocksSavePath, 0755); err != nil {
		log.Fatalf("Failed to create blocks directory: %v", err)
	}
	log.Printf("Storage directory ensured at: %s", cfg.BlocksSavePath)

	monitor, err := NewBlockMonitor(cfg)
	if err != nil {
		log.Fatalf("Failed to create block monitor: %v", err)
	}
	log.Println("Block monitor created successfully")

	// Start the monitor
	if err := monitor.Start(); err != nil {
		log.Fatalf("Failed to start block monitor: %v", err)
	}
	log.Println("Block monitor started successfully")

	// Start HTTP server
	httpServer := api.NewServer(monitor.fs, cfg)
	server := &http.Server{
		Handler:      httpServer.GetRouter(),
		Addr:         fmt.Sprintf("%s:%s", cfg.HTTPHost, cfg.HTTPPort),
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
	}

	go func() {
		log.Printf("Starting HTTP server on %s:%s", cfg.HTTPHost, cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Println("HTTP server started successfully")
	log.Println("Server is ready to handle requests")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	log.Printf("Received shutdown signal: %v", sig)

	log.Println("Initiating graceful shutdown...")

	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server first
	log.Println("Shutting down HTTP server...")
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Then stop the monitor
	log.Println("Stopping block monitor...")
	monitor.Stop()

	log.Println("Shutdown complete")
}
