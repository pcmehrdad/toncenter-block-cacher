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
	client    *api.Client
	config    *config.Config
	validator utils.BlockValidator
	fs        *utils.FileSystem
	stats     struct {
		sync.Mutex
		processedBlocks int
		startTime       time.Time
		lastUpdate      time.Time
		recentBlocks    []string
	}
}

func NewBlockProcessor(cfg *config.Config, validator utils.BlockValidator) *BlockProcessor {
	bp := &BlockProcessor{
		client:    api.NewClient(cfg.BaseURL, cfg.APIKeys, cfg.RateLimitPerKey),
		config:    cfg,
		validator: validator,
		fs:        utils.NewFileSystem(cfg.SavePath, validator),
	}
	bp.stats.recentBlocks = make([]string, 0, 10)
	return bp
}

func (bp *BlockProcessor) updateDisplay() {
	bp.stats.Lock()
	defer bp.stats.Unlock()

	now := time.Now()
	if now.Sub(bp.stats.lastUpdate) >= time.Second {
		elapsed := now.Sub(bp.stats.startTime).Seconds()
		bps := float64(bp.stats.processedBlocks) / elapsed

		// Move to top and clear screen
		fmt.Print("\033[H\033[2J")

		// Print stats
		fmt.Printf("\033[1;36mton-block-processor => Average BPS: %.2f/s\033[0m\n", bps)

		// Print recent blocks
		for _, block := range bp.stats.recentBlocks {
			fmt.Printf("\033[32m%s\033[0m\n", block)
		}

		bp.stats.lastUpdate = now
	}
}

func (bp *BlockProcessor) addBlock(block int) {
	bp.stats.Lock()
	defer bp.stats.Unlock()

	bp.stats.processedBlocks++
	msg := fmt.Sprintf("Completed processing block %d", block)

	if len(bp.stats.recentBlocks) >= 10 {
		// Shift array left
		copy(bp.stats.recentBlocks, bp.stats.recentBlocks[1:])
		bp.stats.recentBlocks[len(bp.stats.recentBlocks)-1] = msg
	} else {
		bp.stats.recentBlocks = append(bp.stats.recentBlocks, msg)
	}
}

func (bp *BlockProcessor) ProcessBlocks(blocks []int) error {
	var wg sync.WaitGroup
	errors := make(chan error, len(blocks))
	semaphore := make(chan struct{}, bp.config.MaxParallelBlocks)

	// Hide cursor
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	// Initialize stats
	bp.stats.processedBlocks = 0
	bp.stats.startTime = time.Now()
	bp.stats.lastUpdate = time.Now()
	bp.stats.recentBlocks = make([]string, 0, 10)

	// Initial display
	fmt.Print("\033[H\033[2J")
	fmt.Printf("\033[1;36mton-block-processor => Average BPS: 0.00/s\033[0m\n")

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

				bp.addBlock(mcSeqno)
				bp.updateDisplay()
				return
			}
		}(block)
	}

	wg.Wait()
	close(errors)

	// Move to bottom of display
	fmt.Printf("\033[%d;1H", 12)

	var errCount int
	for err := range errors {
		fmt.Printf("\033[31mError: %v\033[0m\n", err)
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

		// If no events are returned, break the loop
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

		// If we received fewer events than the limit, we've got all events
		if len(chunk.Chunk.Events) < bp.config.LimitOffset {
			break
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

	return bp.fs.SaveBlockData(mcSeqno, events, addresses)
}
