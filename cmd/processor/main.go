// File: cmd/processor/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
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
				lastProcessedBlock = blockNum
				log.Printf("Processed block %d", blockNum)
			}

			if err := fs.RemoveOldBlocks(cfg.MaxSavedBlocks); err != nil {
				log.Printf("Error cleaning old blocks: %v", err)
			}
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
