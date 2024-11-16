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
	cache    struct {
		sync.RWMutex
		blocks map[int]bool
	}
}

func NewFileSystem(basePath string) *FileSystem {
	fs := &FileSystem{
		basePath: basePath,
	}
	fs.cache.blocks = make(map[int]bool)
	return fs
}

func (fs *FileSystem) GetLastSavedBlockNumber() (uint32, error) {
	// Read the directory where blocks are saved
	entries, err := os.ReadDir(fs.basePath)
	if err != nil {
		return 0, fmt.Errorf("unable to read directory %s: %v", fs.basePath, err)
	}

	var lastBlockNumber uint32

	// Iterate over the directory entries and find the highest block number
	for _, entry := range entries {
		if entry.IsDir() {
			blockNumber, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}

			if uint32(blockNumber) > lastBlockNumber {
				lastBlockNumber = uint32(blockNumber)
			}
		}
	}

	return lastBlockNumber, nil
}

// SaveBlockData saves the block data to the filesystem
func (fs *FileSystem) SaveBlockData(blockNum int, events []byte, addresses []byte) error {
	dirPath := filepath.Join(fs.basePath, strconv.Itoa(blockNum))
	tempDirPath := dirPath + "_temp"

	if err := os.MkdirAll(tempDirPath, 0755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Write files concurrently
	var wg sync.WaitGroup
	var eventErr, addrErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		eventErr = os.WriteFile(filepath.Join(tempDirPath, "events.txt"), events, 0644)
	}()

	go func() {
		defer wg.Done()
		addrErr = os.WriteFile(filepath.Join(tempDirPath, "address_book.txt"), addresses, 0644)
	}()

	wg.Wait()

	if eventErr != nil || addrErr != nil {
		if err := os.RemoveAll(tempDirPath); err != nil {
			fmt.Printf("failed to remove temp directory: %v\n", err)
		}
		if eventErr != nil {
			return fmt.Errorf("save events: %w", eventErr)
		}
		return fmt.Errorf("save addresses: %w", addrErr)
	}

	// Atomically replace the old directory with the new one
	if err := os.RemoveAll(dirPath); err != nil {
		if err := os.RemoveAll(tempDirPath); err != nil {
			fmt.Printf("failed to remove temp directory: %v\n", err)
		}
		return fmt.Errorf("remove old dir: %w", err)
	}

	if err := os.Rename(tempDirPath, dirPath); err != nil {
		if err := os.RemoveAll(tempDirPath); err != nil {
			fmt.Printf("failed to remove temp directory: %v\n", err)
		}
		return fmt.Errorf("rename dir: %w", err)
	}

	// Update cache
	fs.cache.Lock()
	fs.cache.blocks[blockNum] = true
	fs.cache.Unlock()

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

			// Update cache
			fs.cache.Lock()
			delete(fs.cache.blocks, num)
			fs.cache.Unlock()
		}
	}

	return nil
}
