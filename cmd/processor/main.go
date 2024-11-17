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

const (
	BLOCK_LAG   = 5
	MAX_RETRIES = 3
)

type BlockProcessor struct {
	client    *ton.APIClient
	apiClient *api.Client
	fs        *utils.FileSystem
	cfg       *config.Config
	lastBlock int
	mu        sync.RWMutex

	// Track blocks being processed
	processingBlocks sync.Map
}

func NewBlockProcessor(client *ton.APIClient, apiClient *api.Client, fs *utils.FileSystem, cfg *config.Config) *BlockProcessor {
	return &BlockProcessor{
		client:    client,
		apiClient: apiClient,
		fs:        fs,
		cfg:       cfg,
	}
}

func (bp *BlockProcessor) setLastBlock(block int) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.lastBlock = block
}

func (bp *BlockProcessor) getLastBlock() int {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.lastBlock
}

func (bp *BlockProcessor) tryProcessBlock(blockNum int) bool {
	_, loaded := bp.processingBlocks.LoadOrStore(blockNum, true)
	return !loaded
}

func (bp *BlockProcessor) finishProcessing(blockNum int) {
	bp.processingBlocks.Delete(blockNum)
}

func (bp *BlockProcessor) processBlockWithRetry(blockNum int) error {
	// Check if block is already being processed or exists
	if !bp.tryProcessBlock(blockNum) {
		return nil // Block is being processed by another routine
	}
	defer bp.finishProcessing(blockNum)

	// Check if block already exists
	if bp.fs.BlockExists(blockNum) {
		return nil
	}

	var lastErr error
	for retry := 0; retry < MAX_RETRIES; retry++ {
		if err := processBlock(bp.apiClient, bp.fs, blockNum, bp.cfg.FetchLimit); err != nil {
			lastErr = err
			time.Sleep(time.Duration(retry+1) * time.Second)
			continue
		}
		log.Printf("Successfully processed block %d", blockNum)
		return nil
	}
	return lastErr
}

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	client := initTONClient()
	fs := utils.NewFileSystem(cfg.BlocksSavePath)
	apiClient := api.NewClient(
		cfg.ToncenterBaseURL,
		cfg.ToncenterAPIKey,
		cfg.ToncenterRPS,
	)

	if err := os.MkdirAll(cfg.BlocksSavePath, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	processor := NewBlockProcessor(client, apiClient, fs, cfg)

	// Create channels for coordination
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the chain monitoring goroutine
	go processor.monitorChainUpdates(ctx)

	// Start the historical sync goroutine
	go processor.syncHistoricalBlocks(ctx)

	// Start the cleanup goroutine
	go processor.periodicCleanup(ctx)

	// Initialize and start HTTP server
	httpServer := api.NewServer(fs)
	go func() {
		addr := fmt.Sprintf("%s:%s", cfg.HTTPHost, cfg.HTTPPort)
		if err := httpServer.Start(addr); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Handle configuration reload and shutdown
	for sig := range sigChan {
		switch sig {
		case syscall.SIGHUP:
			if newCfg, err := config.LoadConfig(); err != nil {
				log.Printf("Error reloading config: %v", err)
			} else {
				processor.cfg = newCfg
				log.Printf("Configuration reloaded")
			}
		case syscall.SIGINT, syscall.SIGTERM:
			log.Println("Shutting down gracefully...")
			cancel()
			time.Sleep(time.Second) // Give goroutines time to cleanup
			os.Exit(0)
		}
	}
}

func (bp *BlockProcessor) findMissingBlocks(start, end int) []int {
	var missing []int
	for blockNum := start; blockNum <= end; blockNum++ {
		if !bp.fs.BlockExists(blockNum) {
			missing = append(missing, blockNum)
		}
	}
	return missing
}

func (bp *BlockProcessor) monitorChainUpdates(ctx context.Context) {
	log.Println("Starting chain monitor")
	ticker := time.NewTicker(bp.cfg.BlockDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			master, err := bp.client.GetMasterchainInfo(ctx)
			if err != nil {
				log.Printf("Error getting masterchain info: %v", err)
				continue
			}

			currentBlock := int(master.SeqNo) - BLOCK_LAG
			bp.setLastBlock(currentBlock)

			// Process the latest block only if not already being processed
			if err := bp.processBlockWithRetry(currentBlock); err != nil {
				log.Printf("Failed to process latest block %d: %v", currentBlock, err)
			}
		}
	}
}

func (bp *BlockProcessor) syncHistoricalBlocks(ctx context.Context) {
	log.Println("Starting historical sync")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentBlock := bp.getLastBlock()
			if currentBlock == 0 {
				continue // Wait for the chain monitor to get the first block
			}

			startBlock := currentBlock - bp.cfg.MaxSavedBlocks
			if startBlock < 0 {
				startBlock = 0
			}

			// Find all missing blocks in our range
			missingBlocks := bp.findMissingBlocks(startBlock, currentBlock)
			if len(missingBlocks) > 0 {
				log.Printf("Found %d missing blocks in range %d-%d", len(missingBlocks), startBlock, currentBlock)
			}

			// Process missing blocks in batches
			const batchSize = 10
			for i := 0; i < len(missingBlocks); i += batchSize {
				select {
				case <-ctx.Done():
					return
				default:
					end := i + batchSize
					if end > len(missingBlocks) {
						end = len(missingBlocks)
					}

					batch := missingBlocks[i:end]
					var wg sync.WaitGroup
					semaphore := make(chan struct{}, 3) // Limit concurrent processing

					for _, blockNum := range batch {
						wg.Add(1)
						semaphore <- struct{}{} // Acquire semaphore

						go func(num int) {
							defer wg.Done()
							defer func() { <-semaphore }() // Release semaphore

							if err := bp.processBlockWithRetry(num); err != nil {
								log.Printf("Failed to process historical block %d: %v", num, err)
							}
						}(blockNum)
					}

					wg.Wait()
					time.Sleep(bp.cfg.BlockDelay) // Rate limiting between batches
				}
			}
		}
	}
}

func (bp *BlockProcessor) periodicCleanup(ctx context.Context) {
	log.Println("Starting periodic cleanup")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := bp.fs.RemoveOldBlocks(bp.cfg.MaxSavedBlocks); err != nil {
				log.Printf("Error cleaning old blocks: %v", err)
			} else {
				log.Println("Completed periodic cleanup")
			}
		}
	}
}

func initTONClient() *ton.APIClient {
	conn := liteclient.NewConnectionPool()
	err := conn.AddConnectionsFromConfigUrl(context.Background(),
		"https://ton-blockchain.github.io/global.config.json")
	if err != nil {
		log.Fatalf("Failed to initialize TON client: %v", err)
	}
	return ton.NewAPIClient(conn)
}

func processBlock(client *api.Client, fs *utils.FileSystem, blockNum, limit int) error {
	var finalEvents []json.RawMessage
	var finalAddressBook map[string]interface{}
	offset := 0

	for {
		data, eventCount, err := client.FetchChunk(blockNum, limit, offset)
		if err != nil {
			return fmt.Errorf("fetch chunk: %w", err)
		}

		var response struct {
			Events      []json.RawMessage      `json:"events"`
			AddressBook map[string]interface{} `json:"address_book"`
		}

		if err := json.Unmarshal(data, &response); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		finalEvents = append(finalEvents, response.Events...)
		if finalAddressBook == nil {
			finalAddressBook = response.AddressBook
		} else {
			for k, v := range response.AddressBook {
				finalAddressBook[k] = v
			}
		}

		if eventCount < limit {
			break
		}
		offset += limit
	}

	finalResponse := map[string]interface{}{
		"events":       finalEvents,
		"address_book": finalAddressBook,
	}

	combinedData, err := json.MarshalIndent(finalResponse, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal final response: %w", err)
	}

	return fs.SaveBlock(blockNum, combinedData)
}
