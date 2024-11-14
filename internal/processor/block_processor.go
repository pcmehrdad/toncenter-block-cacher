package processor

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ton-block-processor/internal/api"
	"ton-block-processor/internal/config"
	"ton-block-processor/internal/models"
	"ton-block-processor/internal/utils"
)

type BlockProcessor struct {
	client *api.Client
	config *config.Config
}

func NewBlockProcessor(cfg *config.Config) *BlockProcessor {
	return &BlockProcessor{
		client: api.NewClient(cfg.BaseURL, cfg.APIKey, cfg.TotalRateLimit),
		config: cfg,
	}
}

func (bp *BlockProcessor) ProcessBlocks(blocks []int) error {
	var wg sync.WaitGroup
	errors := make(chan error, len(blocks))
	semaphore := make(chan struct{}, bp.config.MaxParallelBlocks)

	for _, block := range blocks {
		time.Sleep(bp.config.BetweenBlockDelay)

		wg.Add(1)
		go func(mcSeqno int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			for retryCount := 0; retryCount < 3; retryCount++ {
				if retryCount > 0 {
					time.Sleep(bp.config.RetryDelay)
				}

				if err := bp.processBlock(mcSeqno); err != nil {
					if retryCount == 2 {
						errors <- fmt.Errorf("block %d: %v", mcSeqno, err)
					}
					continue
				}

				fmt.Printf("Completed processing block %d\n", mcSeqno)
				return
			}
		}(block)
	}

	wg.Wait()
	close(errors)

	var errCount int
	for err := range errors {
		fmt.Printf("Error: %v\n", err)
		errCount++
	}

	if errCount > 0 {
		return fmt.Errorf("%d blocks failed to process", errCount)
	}
	return nil
}

func (bp *BlockProcessor) processBlock(mcSeqno int) error {
	var finalResult models.Response
	offset := 0

	for {
		chunk, err := bp.client.FetchChunk(mcSeqno, bp.config.LimitOffset, offset)
		if err != nil {
			return fmt.Errorf("fetch chunk: %w", err)
		}

		if len(chunk.Chunk.Events) == 0 {
			break
		}

		finalResult.Events = append(finalResult.Events, chunk.Chunk.Events...)
		if finalResult.AddressBook == nil {
			finalResult.AddressBook = make(map[string]models.AddressInfo)
		}
		for addr, info := range chunk.Chunk.AddressBook {
			finalResult.AddressBook[addr] = info
		}

		offset += bp.config.LimitOffset
	}

	events, err := json.Marshal(finalResult.Events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	addresses, err := json.Marshal(finalResult.AddressBook)
	if err != nil {
		return fmt.Errorf("marshal addresses: %w", err)
	}

	fs := utils.NewFileSystem(bp.config.SavePath)
	return fs.SaveBlockData(mcSeqno, events, addresses)
}
