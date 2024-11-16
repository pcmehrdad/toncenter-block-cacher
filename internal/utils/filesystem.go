package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"toncenter-block-cacher/internal/models"
)

type FileSystem struct {
	basePath    string
	mu          sync.RWMutex
	blockCache  map[int]*blockMetadata
	maxCacheAge time.Duration
}

type blockMetadata struct {
	ProcessedAt    time.Time
	LastUpdateTime time.Time
	EventCount     int
}

func NewFileSystem(basePath string) *FileSystem {
	fs := &FileSystem{
		basePath:    basePath,
		blockCache:  make(map[int]*blockMetadata),
		maxCacheAge: 5 * time.Minute,
	}

	// Ensure base directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		panic(fmt.Sprintf("failed to create base directory: %v", err))
	}

	// Load existing blocks into cache
	fs.loadExistingBlocks()

	// Start cache cleanup routine
	go fs.cleanupRoutine()

	return fs
}

func (fs *FileSystem) loadExistingBlocks() {
	entries, err := os.ReadDir(fs.basePath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		blockNum, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		fs.blockCache[blockNum] = &blockMetadata{
			ProcessedAt:    time.Now(),
			LastUpdateTime: time.Now(),
		}
	}
}

func (fs *FileSystem) SaveBlockData(blockNum int, response *models.ChunkResponse) error {
	dirPath := filepath.Join(fs.basePath, strconv.Itoa(blockNum))
	tempDirPath := dirPath + "_temp"

	// Create temp directory
	if err := os.MkdirAll(tempDirPath, 0755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Cleanup function
	cleanup := func() {
		if err := os.RemoveAll(tempDirPath); err != nil {
			fmt.Printf("Failed to remove temp directory: %v\n", err)
		}
	}
	defer cleanup()

	// Prepare data
	events, err := json.MarshalIndent(response.Chunk.Events, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	addresses, err := json.MarshalIndent(response.Chunk.AddressBook, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal addresses: %w", err)
	}

	// Write files
	if err := os.WriteFile(filepath.Join(tempDirPath, "events.json"), events, 0644); err != nil {
		return fmt.Errorf("write events: %w", err)
	}

	if err := os.WriteFile(filepath.Join(tempDirPath, "addresses.json"), addresses, 0644); err != nil {
		return fmt.Errorf("write addresses: %w", err)
	}

	// Atomic directory replacement
	if err := os.RemoveAll(dirPath); err != nil {
		return fmt.Errorf("remove old dir: %w", err)
	}

	if err := os.Rename(tempDirPath, dirPath); err != nil {
		return fmt.Errorf("rename dir: %w", err)
	}

	// Update cache
	fs.mu.Lock()
	fs.blockCache[blockNum] = &blockMetadata{
		ProcessedAt:    time.Now(),
		LastUpdateTime: time.Now(),
		EventCount:     len(response.Chunk.Events),
	}
	fs.mu.Unlock()

	return nil
}

func (fs *FileSystem) GetLastSavedBlockNumber() (uint32, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var lastBlock uint32
	for blockNum := range fs.blockCache {
		if uint32(blockNum) > lastBlock {
			lastBlock = uint32(blockNum)
		}
	}

	if lastBlock == 0 {
		return 0, fmt.Errorf("no blocks found")
	}

	return lastBlock, nil
}

func (fs *FileSystem) RemoveBlocksBefore(blockNum int) error {
	entries, err := os.ReadDir(fs.basePath)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		num, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		if num < blockNum {
			path := filepath.Join(fs.basePath, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove block %d: %w", num, err)
			}

			fs.mu.Lock()
			delete(fs.blockCache, num)
			fs.mu.Unlock()
		}
	}

	return nil
}

func (fs *FileSystem) cleanupRoutine() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		fs.mu.Lock()
		now := time.Now()
		for blockNum, metadata := range fs.blockCache {
			if now.Sub(metadata.LastUpdateTime) > fs.maxCacheAge {
				delete(fs.blockCache, blockNum)
			}
		}
		fs.mu.Unlock()
	}
}
