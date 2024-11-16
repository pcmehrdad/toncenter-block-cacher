package monitor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/ton"
	"toncenter-block-cacher/internal/api"
	"toncenter-block-cacher/internal/config"
	"toncenter-block-cacher/internal/utils"
)

type Monitor struct {
	cfg       *config.Config
	tonClient *ton.APIClient
	api       *api.Client
	fs        *utils.FileSystem

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Statistics
	stats struct {
		sync.RWMutex
		lastBlock      uint32
		processedCount uint64
		failedCount    uint64
		startTime      time.Time
		lastUpdateTime time.Time
		recentBlocks   []string
	}

	// Processing channels
	processingQueue chan uint32
	processedBlocks chan uint32
	errorBlocks     chan uint32
	displayUpdate   chan struct{}
}

func NewMonitor(cfg *config.Config, tonClient *ton.APIClient) (*Monitor, error) {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Monitor{
		cfg:             cfg,
		tonClient:       tonClient, // Dereference the pointer
		api:             api.NewClient(cfg.ToncenterBaseURL, cfg.ToncenterAPIKeys, cfg.ToncenterRPS),
		fs:              utils.NewFileSystem(cfg.BlocksSavePath),
		ctx:             ctx,
		cancel:          cancel,
		processingQueue: make(chan uint32, cfg.MaxParallelFetches*2),
		processedBlocks: make(chan uint32, cfg.MaxParallelFetches),
		errorBlocks:     make(chan uint32, cfg.MaxParallelFetches),
		displayUpdate:   make(chan struct{}, 1),
	}

	m.stats.startTime = time.Now()
	m.stats.recentBlocks = make([]string, 0, 10)

	return m, nil
}

func (m *Monitor) GetFileSystem() *utils.FileSystem {
	return m.fs
}

func (m *Monitor) Start() error {
	log.Println("Starting block monitor...")

	startBlock, err := m.determineStartBlock()
	if err != nil {
		return fmt.Errorf("failed to determine start block: %w", err)
	}

	// Start worker pool
	for i := 0; i < m.cfg.MaxParallelFetches; i++ {
		m.wg.Add(1)
		go m.blockProcessor()
	}

	// Start display updater
	m.wg.Add(1)
	go m.displayUpdater()

	// Start main monitoring loop
	m.wg.Add(1)
	go func() {
		m.monitorBlocks(startBlock)
	}()

	return nil
}

func (m *Monitor) determineStartBlock() (uint32, error) {
	// Get current block from TON
	master, err := m.getCurrentBlock()
	if err != nil {
		return 0, fmt.Errorf("failed to get current block: %w", err)
	}

	// Get last saved block
	lastSaved, err := m.fs.GetLastSavedBlockNumber()
	if err != nil {
		log.Printf("No saved blocks found, starting %d blocks behind current", m.cfg.MaxSavedBlocks)
		if master.SeqNo > uint32(m.cfg.MaxSavedBlocks) {
			return master.SeqNo - uint32(m.cfg.MaxSavedBlocks), nil
		}
		return 0, nil
	}

	// If we're too far behind, start from a more recent block
	if master.SeqNo > lastSaved+uint32(m.cfg.MaxSavedBlocks) {
		return master.SeqNo - uint32(m.cfg.MaxSavedBlocks), nil
	}

	return lastSaved + 1, nil
}

func (m *Monitor) monitorBlocks(startBlock uint32) {
	defer m.wg.Done()
	log.Printf("Starting to monitor blocks from %d", startBlock)

	lastProcessed := startBlock - 1
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			close(m.processingQueue)
			return

		case <-ticker.C:
			master, err := m.getCurrentBlock()
			if err != nil {
				log.Printf("Error getting current block: %v", err)
				time.Sleep(time.Second) // Add delay on error
				continue
			}

			// Stay behind latest block for stability
			safeBlock := master.SeqNo
			if safeBlock > 2 { // Stay 2 blocks behind
				safeBlock -= 2
			}

			// Queue new blocks
			for blockNum := lastProcessed + 1; blockNum <= safeBlock; blockNum++ {
				select {
				case m.processingQueue <- blockNum:
					log.Printf("Queued block %d for processing", blockNum)
					lastProcessed = blockNum
				case <-m.ctx.Done():
					close(m.processingQueue)
					return
				}
			}
		}
	}
}

func (m *Monitor) blockProcessor() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return

		case blockNum, ok := <-m.processingQueue:
			if !ok {
				return
			}

			log.Printf("Processing block %d", blockNum)
			if err := m.processBlockWithRetries(blockNum); err != nil {
				m.stats.Lock()
				m.stats.failedCount++
				m.stats.Unlock()
				log.Printf("Failed to process block %d: %v", blockNum, err)
				continue
			}

			m.updateStats(blockNum)
			time.Sleep(m.cfg.BlockDelay)
		}
	}
}

func (m *Monitor) processBlockWithRetries(blockNum uint32) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := m.processBlock(blockNum); err != nil {
			lastErr = err
			log.Printf("Attempt %d failed for block %d: %v", attempt, blockNum, err)
			select {
			case <-m.ctx.Done():
				return fmt.Errorf("context cancelled")
			case <-time.After(m.cfg.RetryDelay * time.Duration(attempt)):
				continue
			}
		}
		return nil
	}
	return fmt.Errorf("all retries failed: %w", lastErr)
}

func (m *Monitor) processBlock(blockNum uint32) error {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	resp, err := m.api.FetchChunk(ctx, int(blockNum), m.cfg.FetchLimit, 0)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	// Add validation for empty response
	if resp == nil || len(resp.Chunk.Events) == 0 {
		return fmt.Errorf("no events found for block %d", blockNum)
	}

	if err := m.fs.SaveBlockData(int(blockNum), resp); err != nil {
		return fmt.Errorf("save failed: %w", err)
	}

	log.Printf("Successfully processed and saved block %d", blockNum)
	return nil
}

func (m *Monitor) getCurrentBlock() (*ton.BlockIDExt, error) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()

	return m.tonClient.GetMasterchainInfo(ctx)
}

func (m *Monitor) cleanupOldBlocks(currentBlock uint32) error {
	if currentBlock <= uint32(m.cfg.MaxSavedBlocks) {
		return nil
	}

	cutoff := int(currentBlock) - m.cfg.MaxSavedBlocks
	return m.fs.RemoveBlocksBefore(cutoff)
}

func (m *Monitor) updateStats(blockNum uint32) {
	m.stats.Lock()
	defer m.stats.Unlock()

	m.stats.lastBlock = blockNum
	m.stats.processedCount++
	m.stats.lastUpdateTime = time.Now()

	// Update recent blocks list
	msg := fmt.Sprintf("Block %d processed", blockNum)
	if len(m.stats.recentBlocks) >= 10 {
		copy(m.stats.recentBlocks, m.stats.recentBlocks[1:])
		m.stats.recentBlocks[9] = msg
	} else {
		m.stats.recentBlocks = append(m.stats.recentBlocks, msg)
	}
}

func (m *Monitor) displayUpdater() {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateDisplay()
		}
	}
}

func (m *Monitor) updateDisplay() {
	m.stats.RLock()
	defer m.stats.RUnlock()

	elapsed := time.Since(m.stats.startTime).Seconds()
	bps := float64(m.stats.processedCount) / elapsed

	// Clear screen and move cursor to top-left
	fmt.Print("\033[H\033[2J")

	// Print header
	fmt.Printf("\033[1;36mTON Block Cacher - Running for: %.1fs\033[0m\n\n", elapsed)

	// Print stats
	fmt.Printf("Processing Rate: \033[1;32m%.2f blocks/s\033[0m\n", bps)
	fmt.Printf("Last Block: \033[1;33m%d\033[0m\n", m.stats.lastBlock)
	fmt.Printf("Total Processed: \033[1;32m%d\033[0m\n", m.stats.processedCount)
	fmt.Printf("Failed Blocks: \033[1;31m%d\033[0m\n\n", m.stats.failedCount)

	// Print recent blocks
	fmt.Println("\033[1;34mRecent Blocks:\033[0m")
	for _, block := range m.stats.recentBlocks {
		fmt.Printf("\033[32m%s\033[0m\n", block)
	}
}

func (m *Monitor) Stop() {
	log.Println("Stopping monitor...")
	m.cancel()

	// Add timeout for graceful shutdown
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Monitor stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Println("Monitor stop timed out")
	}
}

func (m *Monitor) GetStats() struct {
	LastBlock      uint32
	ProcessedCount uint64
	FailedCount    uint64
	StartTime      time.Time
} {
	m.stats.RLock()
	defer m.stats.RUnlock()

	return struct {
		LastBlock      uint32
		ProcessedCount uint64
		FailedCount    uint64
		StartTime      time.Time
	}{
		LastBlock:      m.stats.lastBlock,
		ProcessedCount: m.stats.processedCount,
		FailedCount:    m.stats.failedCount,
		StartTime:      m.stats.startTime,
	}
}
