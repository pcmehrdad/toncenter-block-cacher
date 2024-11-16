package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

type FileSystem struct {
	basePath string
	mu       sync.RWMutex
	blocks   map[int]bool
}

func NewFileSystem(basePath string) *FileSystem {
	fs := &FileSystem{
		basePath: basePath,
		blocks:   make(map[int]bool),
	}

	// Initialize cache with existing blocks
	if entries, err := os.ReadDir(basePath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				if num, err := strconv.Atoi(entry.Name()); err == nil {
					fs.blocks[num] = true
				}
			}
		}
	}

	return fs
}

func (fs *FileSystem) GetLastSavedBlockNumber() (uint32, error) {
	entries, err := os.ReadDir(fs.basePath)
	if err != nil {
		return 0, fmt.Errorf("unable to read directory %s: %w", fs.basePath, err)
	}

	var lastBlockNumber uint32
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		blockNumber, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		if uint32(blockNumber) > lastBlockNumber {
			lastBlockNumber = uint32(blockNumber)
		}
	}

	return lastBlockNumber, nil
}

func (fs *FileSystem) SaveBlockData(blockNum int, events []byte, addresses []byte) error {
	dirPath := filepath.Join(fs.basePath, strconv.Itoa(blockNum))
	tempDirPath := dirPath + "_temp"

	// Create temp directory
	if err := os.MkdirAll(tempDirPath, 0755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Cleanup function for error cases
	cleanup := func() {
		if err := os.RemoveAll(tempDirPath); err != nil {
			fmt.Printf("failed to remove temp directory: %v\n", err)
		}
	}

	// Write files concurrently
	errChan := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := os.WriteFile(filepath.Join(tempDirPath, "events.txt"), events, 0644); err != nil {
			errChan <- fmt.Errorf("save events: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := os.WriteFile(filepath.Join(tempDirPath, "address_book.txt"), addresses, 0644); err != nil {
			errChan <- fmt.Errorf("save addresses: %w", err)
		}
	}()

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		cleanup()
		return err
	}

	// Atomic replacement
	if err := os.RemoveAll(dirPath); err != nil {
		cleanup()
		return fmt.Errorf("remove old dir: %w", err)
	}

	if err := os.Rename(tempDirPath, dirPath); err != nil {
		cleanup()
		return fmt.Errorf("rename dir: %w", err)
	}

	// Update cache
	fs.mu.Lock()
	fs.blocks[blockNum] = true
	fs.mu.Unlock()

	return nil
}

func (fs *FileSystem) RemoveBlocksBefore(blockNum int) error {
	entries, err := os.ReadDir(fs.basePath)
	if err != nil {
		return fmt.Errorf("read directory failed: %w", err)
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
				return fmt.Errorf("remove block %d failed: %w", num, err)
			}

			fs.mu.Lock()
			delete(fs.blocks, num)
			fs.mu.Unlock()
		}
	}

	return nil
}

// Helper method to check if a block exists
func (fs *FileSystem) BlockExists(blockNum int) bool {
	fs.mu.RLock()
	exists := fs.blocks[blockNum]
	fs.mu.RUnlock()
	return exists
}

func (fs *FileSystem) GetBasePath() string {
	return fs.basePath
}
