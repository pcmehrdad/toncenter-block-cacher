package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	api       *ton.APIClientWrapped
	tonCenter *api.Client
	fs        *utils.FileSystem
	cfg       *config.Config
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

func NewBlockMonitor(cfg *config.Config) (*BlockMonitor, error) {
	// Initialize connection to TON
	conn := liteclient.NewConnectionPool()
	err := conn.AddConnectionsFromConfigUrl(context.Background(), "https://ton-blockchain.github.io/global.config.json")
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}

	// Initialize API clients
	liteApi := ton.NewAPIClient(conn).WithRetry()
	tonCenter := api.NewClient(
		cfg.ApiTonCenterBaseURL,
		[]string{cfg.ApiTonCenterKEY},
		cfg.ApiTonCenterRPS,
	)

	// Initialize filesystem
	fs := utils.NewFileSystem(cfg.BlocksSavePath)

	return &BlockMonitor{
		api:       &liteApi,
		tonCenter: tonCenter,
		fs:        fs,
		cfg:       cfg,
		stopChan:  make(chan struct{}),
	}, nil
}

func (m *BlockMonitor) Start() error {
	// Get last saved block
	lastSavedBlock, err := m.fs.GetLastSavedBlockNumber()
	if err != nil {
		log.Printf("No saved blocks found, starting from current block")
		lastSavedBlock = 0
	}

	// Get current masterchain block
	master, err := m.getMasterchainInfo()
	if err != nil {
		return fmt.Errorf("error getting initial masterchain info: %w", err)
	}

	// If we're too far behind, start from (current - maxSavedBlocks)
	var startBlock uint32
	if lastSavedBlock == 0 || master.SeqNo > lastSavedBlock+uint32(m.cfg.MaxSavedBlocks) {
		startBlock = master.SeqNo - uint32(m.cfg.MaxSavedBlocks)
		log.Printf("Starting from block %d (current: %d, max blocks: %d)",
			startBlock, master.SeqNo, m.cfg.MaxSavedBlocks)
	} else {
		startBlock = lastSavedBlock + 1
		log.Printf("Continuing from last saved block: %d", startBlock)
	}

	// Start the block monitor
	m.wg.Add(1)
	go m.monitorBlocks(startBlock)

	return nil
}

func (m *BlockMonitor) Stop() {
	close(m.stopChan)
	m.wg.Wait()
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

			// Process any new blocks
			for block := lastProcessed + 1; block <= master.SeqNo; block++ {
				select {
				case processingQueue <- block:
					lastProcessed = block
				case <-m.stopChan:
					close(processingQueue)
					processWg.Wait()
					return
				}
			}

			// Clean up old blocks
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
	response, err := m.tonCenter.FetchChunk(int(block), m.cfg.ApiFetchLimit, 0)
	if err != nil {
		return fmt.Errorf("fetch from toncenter failed: %w", err)
	}

	if response == nil || response.Chunk.Events == nil {
		return fmt.Errorf("empty response from toncenter")
	}

	events, err := json.Marshal(response.Chunk.Events)
	if err != nil {
		return fmt.Errorf("marshal events failed: %w", err)
	}

	addresses, err := json.Marshal(response.Chunk.AddressBook)
	if err != nil {
		return fmt.Errorf("marshal addresses failed: %w", err)
	}

	err = m.fs.SaveBlockData(int(block), events, addresses)
	if err != nil {
		return fmt.Errorf("save block failed: %w", err)
	}

	return nil
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

func (m *BlockMonitor) getMasterchainInfo() (*ton.BlockIDExt, error) {
	master, err := (*m.api).GetMasterchainInfo(context.Background())
	if err != nil {
		return nil, fmt.Errorf("get masterchain info: %w", err)
	}
	return master, nil
}

func main() {
	// Initialize logger
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create blocks directory if it doesn't exist
	if err := os.MkdirAll(cfg.BlocksSavePath, 0755); err != nil {
		log.Fatalf("Failed to create blocks directory: %v", err)
	}

	// Create and start block monitor
	monitor, err := NewBlockMonitor(cfg)
	if err != nil {
		log.Fatalf("Failed to create block monitor: %v", err)
	}

	if err := monitor.Start(); err != nil {
		log.Fatalf("Failed to start block monitor: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")
	monitor.Stop()
	log.Println("Shutdown complete")
}
