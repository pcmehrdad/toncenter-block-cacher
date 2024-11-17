// File: cmd/processor/main.go
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
	BLOCK_LAG              = 5
	VERIFY_INTERVAL        = 15 * time.Second // Reduced from 1 minute
	MAX_RETRIES            = 3
	MAX_CONCURRENT_REPAIRS = 10
	BATCH_SIZE             = 100
)

// BlockVerifier handles block verification and repair operations
type BlockVerifier struct {
	fs                *utils.FileSystem
	apiClient         *api.Client
	processedBlocks   map[int]bool
	lastVerifiedBlock map[int]time.Time // Track when each block was last verified
	mutex             sync.RWMutex
	fetchLimit        int
}

func NewBlockVerifier(fs *utils.FileSystem, apiClient *api.Client, fetchLimit int) *BlockVerifier {
	return &BlockVerifier{
		fs:                fs,
		apiClient:         apiClient,
		processedBlocks:   make(map[int]bool),
		lastVerifiedBlock: make(map[int]time.Time),
		fetchLimit:        fetchLimit,
	}
}

func (bv *BlockVerifier) MarkProcessed(blockNum int) {
	bv.mutex.Lock()
	bv.processedBlocks[blockNum] = true
	bv.mutex.Unlock()
}

func (bv *BlockVerifier) VerifyRange(ctx context.Context, startBlock, endBlock int) error {
	now := time.Now()

	// Define verification schedules based on block age
	var blocksToVerify []int

	// Check recent blocks (last hour) every minute
	recentCutoff := endBlock - 3600 // Last hour
	for blockNum := getMax(recentCutoff, startBlock); blockNum <= endBlock; blockNum++ {
		lastVerified, exists := bv.lastVerifiedBlock[blockNum]
		if !exists || now.Sub(lastVerified) > time.Minute {
			blocksToVerify = append(blocksToVerify, blockNum)
		}
	}

	// Check older blocks (1-12 hours old) every 10 minutes
	mediumCutoff := endBlock - 43200 // Last 12 hours
	for blockNum := getMax(mediumCutoff, startBlock); blockNum < recentCutoff; blockNum++ {
		lastVerified, exists := bv.lastVerifiedBlock[blockNum]
		if !exists || now.Sub(lastVerified) > 10*time.Minute {
			blocksToVerify = append(blocksToVerify, blockNum)
		}
	}

	// Check oldest blocks (>12 hours) every hour
	for blockNum := startBlock; blockNum < mediumCutoff; blockNum++ {
		lastVerified, exists := bv.lastVerifiedBlock[blockNum]
		if !exists || now.Sub(lastVerified) > time.Hour {
			blocksToVerify = append(blocksToVerify, blockNum)
		}
	}

	// If no blocks need verification, return early
	if len(blocksToVerify) == 0 {
		return nil
	}

	// Process blocks that need verification
	var (
		wg         sync.WaitGroup
		repairChan = make(chan int, MAX_CONCURRENT_REPAIRS)
		errorChan  = make(chan error, 1)
		sem        = make(chan struct{}, MAX_CONCURRENT_REPAIRS)
	)

	// Start repair workers
	for i := 0; i < MAX_CONCURRENT_REPAIRS; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for blockNum := range repairChan {
				select {
				case sem <- struct{}{}:
					if err := processBlockWithRetry(bv.apiClient, bv.fs, blockNum, bv.fetchLimit); err != nil {
						select {
						case errorChan <- fmt.Errorf("failed to repair block %d: %w", blockNum, err):
						default:
						}
					} else {
						bv.mutex.Lock()
						bv.processedBlocks[blockNum] = true
						bv.lastVerifiedBlock[blockNum] = now
						bv.mutex.Unlock()
						log.Printf("Verified/Repaired block %d", blockNum)
					}
					<-sem
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Send blocks to verification
	for _, blockNum := range blocksToVerify {
		select {
		case <-ctx.Done():
			close(repairChan)
			wg.Wait()
			return ctx.Err()
		default:
			bv.mutex.RLock()
			processed := bv.processedBlocks[blockNum]
			bv.mutex.RUnlock()

			if !processed || !bv.fs.IsBlockValid(blockNum) {
				repairChan <- blockNum
			}
		}
	}

	close(repairChan)
	wg.Wait()

	select {
	case err := <-errorChan:
		return err
	default:
		return nil
	}
}

func (bv *BlockVerifier) CleanOldBlocks(maxAge int) {
	bv.mutex.Lock()
	defer bv.mutex.Unlock()

	for blockNum := range bv.processedBlocks {
		if blockNum < maxAge {
			delete(bv.processedBlocks, blockNum)
			delete(bv.lastVerifiedBlock, blockNum)
		}
	}
}

// Helper function
func getMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	// Channel for graceful shutdown and config reload
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize components
	client := initTONClient()
	fs := utils.NewFileSystem(cfg.BlocksSavePath)
	apiClient := api.NewClient(
		cfg.ToncenterBaseURL,
		cfg.ToncenterAPIKey,
		cfg.ToncenterRPS,
	)
	verifier := NewBlockVerifier(fs, apiClient, cfg.FetchLimit)

	if err := os.MkdirAll(cfg.BlocksSavePath, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	master, err := client.GetMasterchainInfo(context.Background())
	if err != nil {
		log.Fatalf("Failed to get masterchain info: %v", err)
	}

	currentBlock := int(master.SeqNo) - BLOCK_LAG
	startBlock := currentBlock - cfg.MaxSavedBlocks
	if startBlock < 0 {
		startBlock = 0
	}

	// Initial cleanup and sync
	if err := fs.RemoveOldBlocks(cfg.MaxSavedBlocks); err != nil {
		log.Printf("Initial cleanup error: %v", err)
	}

	if err := syncRange(apiClient, fs, startBlock, currentBlock, cfg); err != nil {
		log.Printf("Initial sync error: %v", err)
	}

	// Initialize and start HTTP server
	httpServer := api.NewServer(fs)
	go func() {
		addr := fmt.Sprintf("%s:%s", cfg.HTTPHost, cfg.HTTPPort)
		if err := httpServer.Start(addr); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	var lastProcessedBlock int = currentBlock
	lastVerifyTime := time.Now()

	// Config reload goroutine
	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGHUP:
				if newCfg, err := config.LoadConfig(); err != nil {
					log.Printf("Error reloading config: %v", err)
				} else {
					cfg = newCfg
					if err := fs.RemoveOldBlocks(cfg.MaxSavedBlocks); err != nil {
						log.Printf("Error enforcing new block limit: %v", err)
					}
					log.Printf("Configuration reloaded, enforced max blocks: %d", cfg.MaxSavedBlocks)
				}
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("Shutting down gracefully...")
				os.Exit(0)
			}
		}
	}()

	// Main processing loop
	for {
		master, err := client.GetMasterchainInfo(context.Background())
		if err != nil {
			log.Printf("Error getting masterchain info: %v", err)
			time.Sleep(cfg.RetryDelay)
			continue
		}

		targetBlock := int(master.SeqNo) - BLOCK_LAG
		httpServer.UpdateLastBlock(targetBlock)

		// Process new blocks
		if targetBlock > lastProcessedBlock {
			for blockNum := lastProcessedBlock + 1; blockNum <= targetBlock; blockNum++ {
				if err := processBlockWithRetry(apiClient, fs, blockNum, cfg.FetchLimit); err != nil {
					log.Printf("Failed to process block %d: %v", blockNum, err)
					continue
				}
				verifier.MarkProcessed(blockNum)
				lastProcessedBlock = blockNum
				log.Printf("Processed block %d", blockNum)
			}

			if err := fs.RemoveOldBlocks(cfg.MaxSavedBlocks); err != nil {
				log.Printf("Error cleaning old blocks: %v", err)
			}
		}

		// Periodic verification with improved parallel processing
		if time.Since(lastVerifyTime) >= VERIFY_INTERVAL {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			verifyRange := targetBlock - cfg.MaxSavedBlocks
			if verifyRange < 0 {
				verifyRange = 0
			}

			if err := verifier.VerifyRange(ctx, verifyRange, targetBlock); err != nil {
				log.Printf("Verification error: %v", err)
			}
			cancel()
			lastVerifyTime = time.Now()

			// Cleanup operations
			if err := fs.RemoveOldBlocks(cfg.MaxSavedBlocks); err != nil {
				log.Printf("Error cleaning old blocks: %v", err)
			}
			verifier.CleanOldBlocks(targetBlock - cfg.MaxSavedBlocks)
		}

		time.Sleep(cfg.BlockDelay)
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

func processBlockWithRetry(client *api.Client, fs *utils.FileSystem, blockNum, limit int) error {
	var lastErr error
	for retry := 0; retry < MAX_RETRIES; retry++ {
		if err := processBlock(client, fs, blockNum, limit); err != nil {
			lastErr = err
			time.Sleep(time.Duration(retry+1) * time.Second)
			continue
		}
		return nil
	}
	return lastErr
}

func syncRange(apiClient *api.Client, fs *utils.FileSystem, start, end int, cfg *config.Config) error {
	log.Printf("Syncing blocks from %d to %d", start, end)

	for blockNum := start; blockNum <= end; blockNum++ {
		if err := processBlockWithRetry(apiClient, fs, blockNum, cfg.FetchLimit); err != nil {
			return fmt.Errorf("failed to process block %d: %w", blockNum, err)
		}
		log.Printf("Processed block %d", blockNum)
		time.Sleep(cfg.BlockDelay)
	}

	return nil
}
