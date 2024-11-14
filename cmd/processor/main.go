package main

import (
	"fmt"
	"log"
	"time"

	"ton-block-processor/internal/config"
	"ton-block-processor/internal/processor"
	"ton-block-processor/internal/utils"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// These could also be moved to config if they change frequently
	startBlock := 35119787
	endBlock := 41992365

	blockProcessor := processor.NewBlockProcessor(cfg)
	blocks := getBlocksToProcess(cfg, startBlock, endBlock)

	if len(blocks) == 0 {
		fmt.Println("No blocks to process")
		return
	}

	// Print summary
	fmt.Printf("Total blocks to process: %d\n", len(blocks))
	if len(blocks) > 0 {
		fmt.Printf("First block to process: %d\n", blocks[0])
		fmt.Printf("Last block to process: %d\n", blocks[len(blocks)-1])
	}

	// Process blocks in chunks
	for i := 0; i < len(blocks); i += cfg.ChunkSize {
		end := i + cfg.ChunkSize
		if end > len(blocks) {
			end = len(blocks)
		}

		chunk := blocks[i:end]
		fmt.Printf("Processing blocks %d to %d...\n", chunk[0], chunk[len(chunk)-1])

		if err := blockProcessor.ProcessBlocks(chunk); err != nil {
			fmt.Printf("Error processing blocks %d to %d: %v\n",
				chunk[0], chunk[len(chunk)-1], err)
			time.Sleep(cfg.RetryDelay * 2)
			continue
		}

		time.Sleep(cfg.RetryDelay)
	}

	fmt.Println("Completed processing all blocks")
}

func getBlocksToProcess(cfg *config.Config, startBlock, endBlock int) []int {
	fs := utils.NewFileSystem(cfg.SavePath)
	return fs.GetBlocksToProcess(startBlock, endBlock)
}
