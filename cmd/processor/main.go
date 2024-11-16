// File: cmd/processor/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"

	"toncenter-block-cacher/internal/api"
	"toncenter-block-cacher/internal/config"
	"toncenter-block-cacher/internal/utils"
)

const (
	BLOCK_LAG       = 5
	VERIFY_INTERVAL = 1 * time.Minute
	MAX_RETRIES     = 3
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize TON client
	client := initTONClient()
	fs := utils.NewFileSystem(cfg.BlocksSavePath)
	apiClient := api.NewClient(cfg.ToncenterBaseURL, cfg.ToncenterAPIKey)

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

	// Track processed and verified blocks
	processedBlocks := make(map[int]bool)
	var processedMutex sync.RWMutex
	lastVerifyTime := time.Now()

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
	for {
		master, err := client.GetMasterchainInfo(context.Background())
		if err != nil {
			log.Printf("Error getting masterchain info: %v", err)
			time.Sleep(cfg.RetryDelay)
			continue
		}

		targetBlock := int(master.SeqNo) - BLOCK_LAG
		httpServer.UpdateLastBlock(targetBlock) // Update HTTP server with latest block

		if targetBlock > lastProcessedBlock {
			newBlocks := make([]int, 0, targetBlock-lastProcessedBlock)
			for b := lastProcessedBlock + 1; b <= targetBlock; b++ {
				newBlocks = append(newBlocks, b)
			}

			// Process new blocks
			for _, blockNum := range newBlocks {
				if err := processBlockWithRetry(apiClient, fs, blockNum, cfg.FetchLimit); err != nil {
					log.Printf("Failed to process block %d: %v", blockNum, err)
					continue
				}
				processedMutex.Lock()
				processedBlocks[blockNum] = true
				processedMutex.Unlock()
				lastProcessedBlock = blockNum
				log.Printf("Processed block %d", blockNum)
			}
		}

		// Periodic verification
		if time.Since(lastVerifyTime) >= VERIFY_INTERVAL {
			verifyRange := targetBlock - cfg.MaxSavedBlocks
			if verifyRange < 0 {
				verifyRange = 0
			}

			repairCount := 0
			for blockNum := verifyRange; blockNum <= targetBlock; blockNum++ {
				processedMutex.RLock()
				processed := processedBlocks[blockNum]
				processedMutex.RUnlock()

				if !processed || !fs.IsBlockValid(blockNum) {
					if repairCount >= 5 {
						break
					}
					log.Printf("Repairing block %d", blockNum)
					if err := processBlockWithRetry(apiClient, fs, blockNum, cfg.FetchLimit); err != nil {
						log.Printf("Failed to repair block %d: %v", blockNum, err)
					} else {
						processedMutex.Lock()
						processedBlocks[blockNum] = true
						processedMutex.Unlock()
						repairCount++
					}
				}
			}
			lastVerifyTime = time.Now()
		}

		// Cleanup operations
		if err := fs.RemoveOldBlocks(cfg.MaxSavedBlocks); err != nil {
			log.Printf("Error cleaning old blocks: %v", err)
		}

		processedMutex.Lock()
		for blockNum := range processedBlocks {
			if blockNum < targetBlock-cfg.MaxSavedBlocks {
				delete(processedBlocks, blockNum)
			}
		}
		processedMutex.Unlock()

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

		// Parse response for merging
		var response struct {
			Events      []json.RawMessage      `json:"events"`
			AddressBook map[string]interface{} `json:"address_book"`
		}

		if err := json.Unmarshal(data, &response); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		// Merge events and address book
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

	// Create combined response
	finalResponse := map[string]interface{}{
		"events":       finalEvents,
		"address_book": finalAddressBook,
	}

	// Marshal with indentation
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
